package http

import (
	"errors"
	"net/http"

	"github.com/influxdata/platform"
	kerrors "github.com/influxdata/platform/kit/errors"
	"github.com/influxdata/platform/query"
	"github.com/influxdata/platform/query/influxql"
	"github.com/julienschmidt/httprouter"
)

type InfluxqlQueryHandler struct {
	*httprouter.Router

	QueryService query.QueryService
}

// NewInfluxqlQueryHandler returns a new instance of QueryHandler.
func NewInfluxqlQueryHandler() *InfluxqlQueryHandler {
	h := &InfluxqlQueryHandler{
		Router: httprouter.New(),
	}

	h.HandlerFunc("POST", "/query", h.handlePostQuery)
	return h
}

// handlePostInfluxQL handles query requests mirroring the 1.x influxdb API.
func (h *InfluxqlQueryHandler) handlePostQuery(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	queryStr := r.FormValue("q")
	if queryStr == "" {
		kerrors.EncodeHTTP(ctx, errors.New("must pass query string in q parameter"), w)
		return
	}

	// TODO(jsternberg): This should be a parameter set on the handler itself or be retrieved
	// by some mapping of databases to organization ids instead. The 1.x API doesn't have this
	// so we shouldn't force it to exist.
	var orgID platform.ID
	err := orgID.DecodeFromString(r.FormValue("orgID"))
	if err != nil {
		kerrors.EncodeHTTP(ctx, errors.New("must pass organization ID as string in orgID parameter"), w)
		return
	}

	//TODO(nathanielc): Get database and rp information if needed.

	ce := influxqlCE

	results, err := query.QueryWithTranspile(ctx, orgID, queryStr, h.QueryService, ce.transpiler)
	if err != nil {
		kerrors.EncodeHTTP(ctx, err, w)
		return
	}

	err = encodeResult(w, results, ce.contentType, ce.encoder)
	if err != nil {
		kerrors.EncodeHTTP(ctx, err, w)
		return
	}
}

// crossExecute contains the components needed to execute a transpiled query and encode results
type crossExecute struct {
	transpiler  query.Transpiler
	encoder     query.MultiResultEncoder
	contentType string
}

var influxqlCE = crossExecute{
	transpiler:  influxql.NewTranspiler(),
	encoder:     influxql.NewMultiResultEncoder(),
	contentType: "application/json",
}

func encodeResult(w http.ResponseWriter, results query.ResultIterator, contentType string, encoder query.MultiResultEncoder) error {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)

	return encoder.Encode(w, results)
}
