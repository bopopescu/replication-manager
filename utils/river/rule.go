package river

import (
	"github.com/siddontang/go-mysql/schema"
)

type Rule struct {
	MSchema string `toml:"main_schema"`
	MTable  string `toml:"main_table"`
	CSchema string `toml:"cloud_schema" `
	CTable  string `toml:"cloud_table"`
	Parent  string `toml:"parent"`

	// Default, a MySQL table field name is mapped to Elasticsearch field name.
	// Sometimes, you want to use different name, e.g, the MySQL file name is title,
	// but in Elasticsearch, you want to name it my_title.
	FieldMapping map[string]string `toml:"field"`

	// MySQL table information
	TableInfo *schema.Table
}

func newDefaultRule(main_schema string, main_table string) *Rule {
	r := new(Rule)

	r.MSchema = main_schema
	r.MTable = main_table
	r.CSchema = main_schema
	r.CTable = main_table
	r.FieldMapping = make(map[string]string)

	return r
}

func (r *Rule) prepare() error {
	if r.FieldMapping == nil {
		r.FieldMapping = make(map[string]string)
	}

	if len(r.CSchema) == 0 {
		r.CSchema = r.MTable
	}

	if len(r.CTable) == 0 {
		r.CTable = r.MTable
	}

	return nil
}
