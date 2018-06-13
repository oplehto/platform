package pb

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/influxdata/platform/query"
	"github.com/influxdata/platform/query/execute"
	"github.com/influxdata/platform/query/functions/storage"
	"github.com/influxdata/platform/query/values"
	"github.com/influxdata/yarpc"
	"github.com/pkg/errors"
)

func NewReader(hl storage.HostLookup) (*reader, error) {
	// TODO(nathanielc): Watch for host changes
	hosts := hl.Hosts()
	conns := make([]connection, len(hosts))
	for i, h := range hosts {
		conn, err := yarpc.Dial(h)
		if err != nil {
			return nil, err
		}
		conns[i] = connection{
			host:   h,
			conn:   conn,
			client: NewStorageClient(conn),
		}
	}
	return &reader{
		conns: conns,
	}, nil
}

type reader struct {
	conns []connection
}

type connection struct {
	host   string
	conn   *yarpc.ClientConn
	client StorageClient
}

func (sr *reader) Read(ctx context.Context, trace map[string]string, readSpec storage.ReadSpec, start, stop execute.Time) (query.BlockIterator, error) {
	var predicate *Predicate
	if readSpec.Predicate != nil {
		p, err := ToStoragePredicate(readSpec.Predicate)
		if err != nil {
			return nil, err
		}
		predicate = p
	}

	bi := &bockIterator{
		ctx:   ctx,
		trace: trace,
		bounds: execute.Bounds{
			Start: start,
			Stop:  stop,
		},
		conns:     sr.conns,
		readSpec:  readSpec,
		predicate: predicate,
	}
	return bi, nil
}

func (sr *reader) Close() {
	for _, conn := range sr.conns {
		_ = conn.conn.Close()
	}
}

type bockIterator struct {
	ctx       context.Context
	trace     map[string]string
	bounds    execute.Bounds
	conns     []connection
	readSpec  storage.ReadSpec
	predicate *Predicate
}

func (bi *bockIterator) Do(f func(query.Block) error) error {
	// Setup read request
	var req ReadRequest
	req.Database = string(bi.readSpec.BucketID)
	req.Predicate = bi.predicate
	req.Descending = bi.readSpec.Descending
	req.TimestampRange.Start = int64(bi.bounds.Start)
	req.TimestampRange.End = int64(bi.bounds.Stop)
	req.Group = convertGroupMode(bi.readSpec.GroupMode)
	req.GroupKeys = bi.readSpec.GroupKeys
	req.SeriesLimit = bi.readSpec.SeriesLimit
	req.PointsLimit = bi.readSpec.PointsLimit
	req.SeriesOffset = bi.readSpec.SeriesOffset
	req.Trace = bi.trace

	if req.PointsLimit == -1 {
		req.Hints.SetNoPoints()
	}

	if agg, err := determineAggregateMethod(bi.readSpec.AggregateMethod); err != nil {
		return err
	} else if agg != AggregateTypeNone {
		req.Aggregate = &Aggregate{Type: agg}
	}
	isGrouping := req.Group != GroupAll
	streams := make([]*streamState, 0, len(bi.conns))
	for _, c := range bi.conns {
		if len(bi.readSpec.Hosts) > 0 {
			// Filter down to only hosts provided
			found := false
			for _, h := range bi.readSpec.Hosts {
				if c.host == h {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		stream, err := c.client.Read(bi.ctx, &req)
		if err != nil {
			return err
		}
		streams = append(streams, &streamState{
			bounds:   bi.bounds,
			stream:   stream,
			readSpec: &bi.readSpec,
			group:    isGrouping,
		})
	}

	ms := &mergedStreams{
		streams: streams,
	}

	if isGrouping {
		return bi.handleGroupRead(f, ms)
	}
	return bi.handleRead(f, ms)
}

func (bi *bockIterator) handleRead(f func(query.Block) error, ms *mergedStreams) error {
	for ms.more() {
		if p := ms.peek(); readFrameType(p) != seriesType {
			//This means the consumer didn't read all the data off the block
			return errors.New("internal error: short read")
		}
		frame := ms.next()
		s := frame.GetSeries()
		typ := convertDataType(s.DataType)
		key := partitionKeyForSeries(s, &bi.readSpec, bi.bounds)
		cols, defs := determineBlockColsForSeries(s, typ)
		block := newBlock(bi.bounds, key, cols, ms, &bi.readSpec, s.Tags, defs)

		if err := f(block); err != nil {
			// TODO(nathanielc): Close streams since we have abandoned the request
			return err
		}
		// Wait until the block has been read.
		block.wait()
	}
	return nil
}

func (bi *bockIterator) handleGroupRead(f func(query.Block) error, ms *mergedStreams) error {
	for ms.more() {
		if p := ms.peek(); readFrameType(p) != groupType {
			//This means the consumer didn't read all the data off the block
			return errors.New("internal error: short read")
		}
		frame := ms.next()
		s := frame.GetGroup()
		key := partitionKeyForGroup(s, &bi.readSpec, bi.bounds)

		// try to infer type
		// TODO(sgc): this is a hack
		typ := query.TString
		if p := ms.peek(); readFrameType(p) == seriesType {
			typ = convertDataType(p.GetSeries().DataType)
		}
		cols, defs := determineBlockColsForGroup(s, typ)

		block := newBlock(bi.bounds, key, cols, ms, &bi.readSpec, nil, defs)

		if err := f(block); err != nil {
			// TODO(nathanielc): Close streams since we have abandoned the request
			return err
		}
		// Wait until the block has been read.
		block.wait()
	}
	return nil
}

func determineAggregateMethod(agg string) (Aggregate_AggregateType, error) {
	if agg == "" {
		return AggregateTypeNone, nil
	}

	if t, ok := Aggregate_AggregateType_value[strings.ToUpper(agg)]; ok {
		return Aggregate_AggregateType(t), nil
	}
	return 0, fmt.Errorf("unknown aggregate type %q", agg)
}

func convertGroupMode(m storage.GroupMode) ReadRequest_Group {
	switch m {
	case storage.GroupModeNone:
		return GroupNone
	case storage.GroupModeBy:
		return GroupBy
	case storage.GroupModeExcept:
		return GroupExcept

	case storage.GroupModeDefault, storage.GroupModeAll:
		fallthrough
	default:
		return GroupAll
	}
}

func convertDataType(t ReadResponse_DataType) query.DataType {
	switch t {
	case DataTypeFloat:
		return query.TFloat
	case DataTypeInteger:
		return query.TInt
	case DataTypeUnsigned:
		return query.TUInt
	case DataTypeBoolean:
		return query.TBool
	case DataTypeString:
		return query.TString
	default:
		return query.TInvalid
	}
}

const (
	startColIdx = 0
	stopColIdx  = 1
	timeColIdx  = 2
	valueColIdx = 3
)

func determineBlockColsForSeries(s *ReadResponse_SeriesFrame, typ query.DataType) ([]query.ColMeta, [][]byte) {
	cols := make([]query.ColMeta, 4+len(s.Tags))
	defs := make([][]byte, 4+len(s.Tags))
	cols[startColIdx] = query.ColMeta{
		Label: execute.DefaultStartColLabel,
		Type:  query.TTime,
	}
	cols[stopColIdx] = query.ColMeta{
		Label: execute.DefaultStopColLabel,
		Type:  query.TTime,
	}
	cols[timeColIdx] = query.ColMeta{
		Label: execute.DefaultTimeColLabel,
		Type:  query.TTime,
	}
	cols[valueColIdx] = query.ColMeta{
		Label: execute.DefaultValueColLabel,
		Type:  typ,
	}
	for j, tag := range s.Tags {
		cols[4+j] = query.ColMeta{
			Label: string(tag.Key),
			Type:  query.TString,
		}
		defs[4+j] = []byte("")
	}
	return cols, defs
}

func partitionKeyForSeries(s *ReadResponse_SeriesFrame, readSpec *storage.ReadSpec, bnds execute.Bounds) query.PartitionKey {
	cols := make([]query.ColMeta, 2, len(s.Tags))
	vs := make([]values.Value, 2, len(s.Tags))
	cols[0] = query.ColMeta{
		Label: execute.DefaultStartColLabel,
		Type:  query.TTime,
	}
	vs[0] = values.NewTimeValue(bnds.Start)
	cols[1] = query.ColMeta{
		Label: execute.DefaultStopColLabel,
		Type:  query.TTime,
	}
	vs[1] = values.NewTimeValue(bnds.Stop)
	switch readSpec.GroupMode {
	case storage.GroupModeBy:
		// partition key in GroupKeys order, including tags in the GroupKeys slice
		for _, k := range readSpec.GroupKeys {
			if i := indexOfTag(s.Tags, k); i < len(s.Tags) {
				cols = append(cols, query.ColMeta{
					Label: string(s.Tags[i].Key),
					Type:  query.TString,
				})
				vs = append(vs, values.NewStringValue(string(s.Tags[i].Value)))
			}
		}
	case storage.GroupModeExcept:
		// partition key in GroupKeys order, skipping tags in the GroupKeys slice
		for _, k := range readSpec.GroupKeys {
			if i := indexOfTag(s.Tags, k); i == len(s.Tags) {
				cols = append(cols, query.ColMeta{
					Label: string(s.Tags[i].Key),
					Type:  query.TString,
				})
				vs = append(vs, values.NewStringValue(string(s.Tags[i].Value)))
			}
		}
	case storage.GroupModeDefault, storage.GroupModeAll:
		for i := range s.Tags {
			cols = append(cols, query.ColMeta{
				Label: string(s.Tags[i].Key),
				Type:  query.TString,
			})
			vs = append(vs, values.NewStringValue(string(s.Tags[i].Value)))
		}
	}
	return execute.NewPartitionKey(cols, vs)
}

func determineBlockColsForGroup(f *ReadResponse_GroupFrame, typ query.DataType) ([]query.ColMeta, [][]byte) {
	cols := make([]query.ColMeta, 4+len(f.TagKeys))
	defs := make([][]byte, 4+len(f.TagKeys))
	cols[startColIdx] = query.ColMeta{
		Label: execute.DefaultStartColLabel,
		Type:  query.TTime,
	}
	cols[stopColIdx] = query.ColMeta{
		Label: execute.DefaultStopColLabel,
		Type:  query.TTime,
	}
	cols[timeColIdx] = query.ColMeta{
		Label: execute.DefaultTimeColLabel,
		Type:  query.TTime,
	}
	cols[valueColIdx] = query.ColMeta{
		Label: execute.DefaultValueColLabel,
		Type:  typ,
	}
	for j, tag := range f.TagKeys {
		cols[4+j] = query.ColMeta{
			Label: string(tag),
			Type:  query.TString,
		}
		defs[4+j] = []byte("")

	}
	return cols, defs
}

func partitionKeyForGroup(g *ReadResponse_GroupFrame, readSpec *storage.ReadSpec, bnds execute.Bounds) query.PartitionKey {
	cols := make([]query.ColMeta, 2, len(readSpec.GroupKeys)+2)
	vs := make([]values.Value, 2, len(readSpec.GroupKeys)+2)
	cols[0] = query.ColMeta{
		Label: execute.DefaultStartColLabel,
		Type:  query.TTime,
	}
	vs[0] = values.NewTimeValue(bnds.Start)
	cols[1] = query.ColMeta{
		Label: execute.DefaultStopColLabel,
		Type:  query.TTime,
	}
	vs[1] = values.NewTimeValue(bnds.Stop)
	for i := range readSpec.GroupKeys {
		cols = append(cols, query.ColMeta{
			Label: readSpec.GroupKeys[i],
			Type:  query.TString,
		})
		vs = append(vs, values.NewStringValue(string(g.PartitionKeyVals[i])))
	}
	return execute.NewPartitionKey(cols, vs)
}

// block implement OneTimeBlock as it can only be read once.
// Since it can only be read once it is also a ValueIterator for itself.
type block struct {
	bounds execute.Bounds
	key    query.PartitionKey
	cols   []query.ColMeta

	empty bool
	more  bool

	// cache of the tags on the current series.
	// len(tags) == len(colMeta)
	tags [][]byte
	defs [][]byte

	readSpec *storage.ReadSpec

	done chan struct{}

	ms *mergedStreams

	// The current number of records in memory
	l int
	// colBufs are the buffers for the given columns.
	colBufs []interface{}

	// resuable buffer for the time column
	timeBuf []execute.Time

	// resuable buffers for the different types of values
	boolBuf   []bool
	intBuf    []int64
	uintBuf   []uint64
	floatBuf  []float64
	stringBuf []string

	err error
}

func newBlock(
	bounds execute.Bounds,
	key query.PartitionKey,
	cols []query.ColMeta,
	ms *mergedStreams,
	readSpec *storage.ReadSpec,
	tags []Tag,
	defs [][]byte,
) *block {
	b := &block{
		bounds:   bounds,
		key:      key,
		tags:     make([][]byte, len(cols)),
		defs:     defs,
		colBufs:  make([]interface{}, len(cols)),
		cols:     cols,
		readSpec: readSpec,
		ms:       ms,
		done:     make(chan struct{}),
		empty:    true,
	}
	b.readTags(tags)
	// Call advance now so that we know if we are empty or not
	b.more = b.advance()
	return b
}

func (b *block) RefCount(n int) {
	//TODO(nathanielc): Have the storageBlock consume the Allocator,
	// once we have zero-copy serialization over the network
}

func (b *block) Err() error { return b.err }

func (b *block) wait() {
	<-b.done
}

func (b *block) Key() query.PartitionKey {
	return b.key
}
func (b *block) Cols() []query.ColMeta {
	return b.cols
}

// onetime satisfies the OneTimeBlock interface since this block may only be read once.
func (b *block) onetime() {}
func (b *block) Do(f func(query.ColReader) error) error {
	defer close(b.done)
	// If the initial advance call indicated we are done, return immediately
	if !b.more {
		return b.err
	}

	f(b)
	for b.advance() {
		if err := f(b); err != nil {
			return err
		}
	}
	return b.err
}

func (b *block) Len() int {
	return b.l
}

func (b *block) Bools(j int) []bool {
	execute.CheckColType(b.cols[j], query.TBool)
	return b.colBufs[j].([]bool)
}
func (b *block) Ints(j int) []int64 {
	execute.CheckColType(b.cols[j], query.TInt)
	return b.colBufs[j].([]int64)
}
func (b *block) UInts(j int) []uint64 {
	execute.CheckColType(b.cols[j], query.TUInt)
	return b.colBufs[j].([]uint64)
}
func (b *block) Floats(j int) []float64 {
	execute.CheckColType(b.cols[j], query.TFloat)
	return b.colBufs[j].([]float64)
}
func (b *block) Strings(j int) []string {
	execute.CheckColType(b.cols[j], query.TString)
	return b.colBufs[j].([]string)
}
func (b *block) Times(j int) []execute.Time {
	execute.CheckColType(b.cols[j], query.TTime)
	return b.colBufs[j].([]execute.Time)
}

// readTags populates b.tags with the provided tags
func (b *block) readTags(tags []Tag) {
	for j := range b.tags {
		b.tags[j] = b.defs[j]
	}

	if len(tags) == 0 {
		return
	}

	for _, t := range tags {
		k := string(t.Key)
		j := execute.ColIdx(k, b.cols)
		b.tags[j] = t.Value
	}
}

func (b *block) advance() bool {
	for b.ms.more() {
		//reset buffers
		b.timeBuf = b.timeBuf[0:0]
		b.boolBuf = b.boolBuf[0:0]
		b.intBuf = b.intBuf[0:0]
		b.uintBuf = b.uintBuf[0:0]
		b.stringBuf = b.stringBuf[0:0]
		b.floatBuf = b.floatBuf[0:0]

		switch p := b.ms.peek(); readFrameType(p) {
		case groupType:
			return false
		case seriesType:
			if !b.ms.key().Equal(b.key) {
				// We have reached the end of data for this block
				return false
			}
			s := p.GetSeries()
			b.readTags(s.Tags)

			// Advance to next frame
			b.ms.next()

			if b.readSpec.PointsLimit == -1 {
				// do not expect points frames
				b.l = 0
				return true
			}
		case boolPointsType:
			if b.cols[valueColIdx].Type != query.TBool {
				b.err = fmt.Errorf("value type changed from %s -> %s", b.cols[valueColIdx].Type, query.TBool)
				// TODO: Add error handling
				// Type changed,
				return false
			}
			b.empty = false
			// read next frame
			frame := b.ms.next()
			p := frame.GetBooleanPoints()
			l := len(p.Timestamps)
			b.l = l
			if l > cap(b.timeBuf) {
				b.timeBuf = make([]execute.Time, l)
			} else {
				b.timeBuf = b.timeBuf[:l]
			}
			if l > cap(b.boolBuf) {
				b.boolBuf = make([]bool, l)
			} else {
				b.boolBuf = b.boolBuf[:l]
			}

			for i, c := range p.Timestamps {
				b.timeBuf[i] = execute.Time(c)
				b.boolBuf[i] = p.Values[i]
			}
			b.colBufs[timeColIdx] = b.timeBuf
			b.colBufs[valueColIdx] = b.boolBuf
			b.appendTags()
			b.appendBounds()
			return true
		case intPointsType:
			if b.cols[valueColIdx].Type != query.TInt {
				b.err = fmt.Errorf("value type changed from %s -> %s", b.cols[valueColIdx].Type, query.TInt)
				// TODO: Add error handling
				// Type changed,
				return false
			}
			b.empty = false
			// read next frame
			frame := b.ms.next()
			p := frame.GetIntegerPoints()
			l := len(p.Timestamps)
			b.l = l
			if l > cap(b.timeBuf) {
				b.timeBuf = make([]execute.Time, l)
			} else {
				b.timeBuf = b.timeBuf[:l]
			}
			if l > cap(b.uintBuf) {
				b.intBuf = make([]int64, l)
			} else {
				b.intBuf = b.intBuf[:l]
			}

			for i, c := range p.Timestamps {
				b.timeBuf[i] = execute.Time(c)
				b.intBuf[i] = p.Values[i]
			}
			b.colBufs[timeColIdx] = b.timeBuf
			b.colBufs[valueColIdx] = b.intBuf
			b.appendTags()
			b.appendBounds()
			return true
		case uintPointsType:
			if b.cols[valueColIdx].Type != query.TUInt {
				b.err = fmt.Errorf("value type changed from %s -> %s", b.cols[valueColIdx].Type, query.TUInt)
				// TODO: Add error handling
				// Type changed,
				return false
			}
			b.empty = false
			// read next frame
			frame := b.ms.next()
			p := frame.GetUnsignedPoints()
			l := len(p.Timestamps)
			b.l = l
			if l > cap(b.timeBuf) {
				b.timeBuf = make([]execute.Time, l)
			} else {
				b.timeBuf = b.timeBuf[:l]
			}
			if l > cap(b.intBuf) {
				b.uintBuf = make([]uint64, l)
			} else {
				b.uintBuf = b.uintBuf[:l]
			}

			for i, c := range p.Timestamps {
				b.timeBuf[i] = execute.Time(c)
				b.uintBuf[i] = p.Values[i]
			}
			b.colBufs[timeColIdx] = b.timeBuf
			b.colBufs[valueColIdx] = b.uintBuf
			b.appendTags()
			b.appendBounds()
			return true
		case floatPointsType:
			if b.cols[valueColIdx].Type != query.TFloat {
				b.err = fmt.Errorf("value type changed from %s -> %s", b.cols[valueColIdx].Type, query.TFloat)
				// TODO: Add error handling
				// Type changed,
				return false
			}
			b.empty = false
			// read next frame
			frame := b.ms.next()
			p := frame.GetFloatPoints()

			l := len(p.Timestamps)
			b.l = l
			if l > cap(b.timeBuf) {
				b.timeBuf = make([]execute.Time, l)
			} else {
				b.timeBuf = b.timeBuf[:l]
			}
			if l > cap(b.floatBuf) {
				b.floatBuf = make([]float64, l)
			} else {
				b.floatBuf = b.floatBuf[:l]
			}

			for i, c := range p.Timestamps {
				b.timeBuf[i] = execute.Time(c)
				b.floatBuf[i] = p.Values[i]
			}
			b.colBufs[timeColIdx] = b.timeBuf
			b.colBufs[valueColIdx] = b.floatBuf
			b.appendTags()
			b.appendBounds()
			return true
		case stringPointsType:
			if b.cols[valueColIdx].Type != query.TString {
				b.err = fmt.Errorf("value type changed from %s -> %s", b.cols[valueColIdx].Type, query.TString)
				// TODO: Add error handling
				// Type changed,
				return false
			}
			b.empty = false
			// read next frame
			frame := b.ms.next()
			p := frame.GetStringPoints()

			l := len(p.Timestamps)
			b.l = l
			if l > cap(b.timeBuf) {
				b.timeBuf = make([]execute.Time, l)
			} else {
				b.timeBuf = b.timeBuf[:l]
			}
			if l > cap(b.stringBuf) {
				b.stringBuf = make([]string, l)
			} else {
				b.stringBuf = b.stringBuf[:l]
			}

			for i, c := range p.Timestamps {
				b.timeBuf[i] = execute.Time(c)
				b.stringBuf[i] = p.Values[i]
			}
			b.colBufs[timeColIdx] = b.timeBuf
			b.colBufs[valueColIdx] = b.stringBuf
			b.appendTags()
			b.appendBounds()
			return true
		}
	}
	return false
}

// appendTags fills the colBufs for the tag columns with the tag value.
func (b *block) appendTags() {
	for j := range b.cols {
		v := b.tags[j]
		if v != nil {
			if b.colBufs[j] == nil {
				b.colBufs[j] = make([]string, b.l)
			}
			colBuf := b.colBufs[j].([]string)
			if cap(colBuf) < b.l {
				colBuf = make([]string, b.l)
			} else {
				colBuf = colBuf[:b.l]
			}
			vStr := string(v)
			for i := range colBuf {
				colBuf[i] = vStr
			}
			b.colBufs[j] = colBuf
		}
	}
}

// appendBounds fills the colBufs for the time bounds
func (b *block) appendBounds() {
	bounds := []execute.Time{b.bounds.Start, b.bounds.Stop}
	for j := range []int{startColIdx, stopColIdx} {
		if b.colBufs[j] == nil {
			b.colBufs[j] = make([]execute.Time, b.l)
		}
		colBuf := b.colBufs[j].([]execute.Time)
		if cap(colBuf) < b.l {
			colBuf = make([]execute.Time, b.l)
		} else {
			colBuf = colBuf[:b.l]
		}
		for i := range colBuf {
			colBuf[i] = bounds[j]
		}
		b.colBufs[j] = colBuf
	}
}

func (b *block) Empty() bool {
	return b.empty
}

type streamState struct {
	bounds     execute.Bounds
	stream     Storage_ReadClient
	rep        ReadResponse
	currentKey query.PartitionKey
	readSpec   *storage.ReadSpec
	finished   bool
	group      bool
}

func (s *streamState) peek() ReadResponse_Frame {
	return s.rep.Frames[0]
}

func (s *streamState) more() bool {
	if s.finished {
		return false
	}
	if len(s.rep.Frames) > 0 {
		return true
	}
	if err := s.stream.RecvMsg(&s.rep); err != nil {
		s.finished = true
		if err == io.EOF {
			// We are done
			return false
		}
		//TODO add proper error handling
		return false
	}
	if len(s.rep.Frames) == 0 {
		return false
	}
	s.computeKey()
	return true
}

func (s *streamState) key() query.PartitionKey {
	return s.currentKey
}

func (s *streamState) computeKey() {
	// Determine new currentKey
	p := s.peek()
	ft := readFrameType(p)
	if s.group {
		if ft == groupType {
			group := p.GetGroup()
			s.currentKey = partitionKeyForGroup(group, s.readSpec, s.bounds)
		}
	} else {
		if ft == seriesType {
			series := p.GetSeries()
			s.currentKey = partitionKeyForSeries(series, s.readSpec, s.bounds)
		}
	}
}

func (s *streamState) next() ReadResponse_Frame {
	frame := s.rep.Frames[0]
	s.rep.Frames = s.rep.Frames[1:]
	if len(s.rep.Frames) > 0 {
		s.computeKey()
	}
	return frame
}

type mergedStreams struct {
	streams    []*streamState
	currentKey query.PartitionKey
	i          int
}

func (s *mergedStreams) key() query.PartitionKey {
	if len(s.streams) == 1 {
		return s.streams[0].key()
	}
	return s.currentKey
}
func (s *mergedStreams) peek() ReadResponse_Frame {
	return s.streams[s.i].peek()
}

func (s *mergedStreams) next() ReadResponse_Frame {
	return s.streams[s.i].next()
}

func (s *mergedStreams) more() bool {
	// Optimze for the case of just one stream
	if len(s.streams) == 1 {
		return s.streams[0].more()
	}
	if s.i < 0 {
		return false
	}
	if s.currentKey == nil {
		return s.determineNewKey()
	}
	if s.streams[s.i].more() {
		if s.streams[s.i].key().Equal(s.currentKey) {
			return true
		}
		return s.advance()
	}
	return s.advance()
}

func (s *mergedStreams) advance() bool {
	s.i++
	if s.i == len(s.streams) {
		if !s.determineNewKey() {
			// no new data on any stream
			return false
		}
	}
	return s.more()
}

func (s *mergedStreams) determineNewKey() bool {
	minIdx := -1
	var minKey query.PartitionKey
	for i, stream := range s.streams {
		if !stream.more() {
			continue
		}
		k := stream.key()
		if i == 0 || k.Less(minKey) {
			minIdx = i
			minKey = k
		}
	}
	s.currentKey = minKey
	s.i = minIdx
	return s.i >= 0
}

type frameType int

const (
	seriesType frameType = iota
	groupType
	boolPointsType
	intPointsType
	uintPointsType
	floatPointsType
	stringPointsType
)

func readFrameType(frame ReadResponse_Frame) frameType {
	switch frame.Data.(type) {
	case *ReadResponse_Frame_Series:
		return seriesType
	case *ReadResponse_Frame_Group:
		return groupType
	case *ReadResponse_Frame_BooleanPoints:
		return boolPointsType
	case *ReadResponse_Frame_IntegerPoints:
		return intPointsType
	case *ReadResponse_Frame_UnsignedPoints:
		return uintPointsType
	case *ReadResponse_Frame_FloatPoints:
		return floatPointsType
	case *ReadResponse_Frame_StringPoints:
		return stringPointsType
	default:
		panic(fmt.Errorf("unknown read response frame type: %T", frame.Data))
	}
}

func indexOfTag(t []Tag, k string) int {
	return sort.Search(len(t), func(i int) bool { return string(t[i].Key) >= k })
}
