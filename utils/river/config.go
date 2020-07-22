package river

import (
	"io/ioutil"

	"github.com/BurntSushi/toml"
	"github.com/juju/errors"
)

type SourceConfig struct {
	Schema string   `toml:"schema"`
	Tables []string `toml:"tables"`
}
type Index struct {
	Sql         string `toml:"sql"`
	Triggers    string `toml:"triggers"`
	CloudTable  string `toml:"subordinate_table"`
	CloudSchema string `toml:"subordinate_schema"`
}

type Config struct {
	MyHost        string `toml:"main_host"`
	MyUser        string `toml:"main_user"`
	MyPassword    string `toml:"main_password"`
	SubordinateHost     string `toml:"subordinate_host"`
	SubordinateUser     string `toml:"subordinate_user"`
	SubordinatePassword string `toml:"subordinate_password"`

	MyFlavor string `toml:"main_flavor"`

	DumpPath     string         `toml:"dump_path"`
	DumpServerID uint32         `toml:"dump_server_id"`
	DumpExec     string         `toml:"dump_exec"`
	DumpThreads  uint32         `toml:"dump_threads"`
	DumpInit     bool           `toml:"init"`
	DumpOnly     bool           `toml:"dump_only"`
	BatchMode    string         `toml:"batch_mode"`
	BatchSize    int64          `toml:"batch_size"`
	BatchTimeOut int64          `toml:"batch_timeout"`
	StatAddr     string         `toml:"stat_addr"`
	DataDir      string         `toml:"data_dir"`
	Sources      []SourceConfig `toml:"source"`

	Rules []*Rule  `toml:"rule"`
	index []*Index `toml:"index"`
}

func NewConfigWithFile(name string) (*Config, error) {
	data, err := ioutil.ReadFile(name)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return NewConfig(string(data))
}

func NewConfig(data string) (*Config, error) {
	var c Config

	_, err := toml.Decode(data, &c)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &c, nil
}
