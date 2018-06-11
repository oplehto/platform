package influxql

type Config struct {
	Database        string `json:"db"`
	RetentionPolicy string `json:"rp"`
}

func (t *Transpiler) DefaultConfig() interface{} {
	return &Config{}
}
