package conf

type Bootstrap struct {
	Server   *Server   `yaml:"server"`
	Data     *Data     `yaml:"data"`
	Platform *Platform `yaml:"platform"`
}

type Server struct {
	Http *Server_HTTP `yaml:"http"`
	Grpc *Server_GRPC `yaml:"grpc"`
}

type Server_HTTP struct {
	Addr    string `yaml:"addr"`
	Timeout int    `yaml:"timeout"` // seconds
}

type Server_GRPC struct {
	Addr    string `yaml:"addr"`
	Timeout int    `yaml:"timeout"` // seconds
}

type Data struct {
	Database *Data_Database `yaml:"database"`
	Redis    *Data_Redis    `yaml:"redis"`
}

type Data_Database struct {
	Driver string `yaml:"driver"`
	DSN    string `yaml:"dsn"`
}

type Data_Redis struct {
	Addr         string `yaml:"addr"`
	ReadTimeout  int    `yaml:"read_timeout"`  // seconds
	WriteTimeout int    `yaml:"write_timeout"` // seconds
}

type Platform struct {
	Ickey  *PlatformConf `yaml:"ickey"`
	Szlcsc *PlatformConf `yaml:"szlcsc"`
	Icgoo  *PlatformConf `yaml:"icgoo"`
}

type PlatformConf struct {
	SearchURL     string `yaml:"search_url"`
	Timeout       int    `yaml:"timeout"`
	CrawlerPath   string `yaml:"crawler_path"`
	CrawlerScript string `yaml:"crawler_script"`
}
