package yammy_test

import (
	"fmt"

	. "github.com/yuin/yammy"
)

var exampleFS = newMockFS(map[string][]byte{
	"config.yml": []byte(`
_directives:
  include:
    - child.yml
name: aaa
`),
	"child.yml": []byte(`
child:
  value: bbb
  arr:
    - 10
    - 20
    - 30
`),
})

type Config struct {
	SourceMap *SourceMap `yaml:"sourcemap"`
	Name      string
	Child     ChildConfig
}

func (c *Config) Validate() []error {
	var errs []error
	if len(c.Name) < 5 {
		cm := c.SourceMap.FindMap("/name")
		errs = append(errs, fmt.Errorf("%s (%s:%d) must be longer than 4",
			cm.Path, cm.File, cm.Line))
	}
	cm := c.SourceMap.FindMap("/child")
	errs = append(errs, c.Child.validate(c.SourceMap, cm)...)
	return errs
}

type ChildConfig struct {
	Value string
	Arr   []int
}

func (c *ChildConfig) validate(sm *SourceMap, m *Mapping) []error {
	var errs []error
	if len(c.Value) < 5 {
		cm := sm.FindMap(fmt.Sprintf("%s/%s", m.Path, "value"))
		errs = append(errs, fmt.Errorf("%s (%s:%d) must be longer than 4",
			cm.Path, cm.File, cm.Line))
	}
	for i, v := range c.Arr {
		if v < 21 {
			cm := sm.FindMap(fmt.Sprintf("%s/arr/%d", m.Path, i))
			errs = append(errs, fmt.Errorf("%s (%s:%d) must be bigger than 20",
				cm.Path, cm.File, cm.Line))
		}
	}
	return errs
}

func ExampleLoad() {
	var c Config
	err := Load("config.yml",
		&c,
		WithFileSystem(exampleFS),
		WithSourceMapKey("sourcemap"))
	if err != nil {
		panic(err)
	}
	errs := c.Validate()
	for _, err := range errs {
		fmt.Println(err.Error())
	}

	// Output:
	// /name (config.yml:5) must be longer than 4
	// /child/value (child.yml:3) must be longer than 4
	// /child/arr/0 (child.yml:5) must be bigger than 20
	// /child/arr/1 (child.yml:6) must be bigger than 20
}
