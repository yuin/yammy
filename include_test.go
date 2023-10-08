package yammy_test

import (
	"encoding/json"
	"testing"

	. "github.com/yuin/yammy"
)

func TestInclude(t *testing.T) {

	fs := newMockFS(map[string][]byte{
		"test.yml": []byte(`
_directives:
  include:
    - child.yml
test:
  value: 10
  c1: 99
  value2:
    - 20
`),
		"child.yml": []byte(`
_directives:
  include:
    - grand-child.yml
    - grand-child2.yml
child:
  c2: 100
  value3:
    - 30
    - 40
test:
  c1: 999
  value2:
    - 40
`),
		"grand-child.yml": []byte(`
grand:
  value4: aaa
child:
  c3: 777
  value3:
    - 30
    - 60
test:
  value2:
    - 70
`),
		"grand-child2.yml": []byte(`
grand:
  value4: bbb
  value5: ccc
`),
	})
	var result map[string]any
	err := Load("test.yml", &result, WithFileSystem(fs))
	if err != nil {
		t.Error(err.Error())
	}

	bs, err := json.Marshal(result)
	if err != nil {
		t.Error(err.Error())
	}
	expected := `{"child":{"c2":100,"c3":777,"value3":[30,60,30,40]},"grand":` +
		`{"value4":"bbb","value5":"ccc"},"test":{"c1":99,"value":10,"value2":[70,40,20]}}`
	if expected != string(bs) {
		t.Errorf("expected:\n--------------\n%s\n\nactual:\n---------------\n%s\n",
			expected, string(bs))
	}
}
