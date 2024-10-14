package yammy_test

import (
	"testing"

	. "github.com/yuin/yammy"
	"gopkg.in/yaml.v3"
)

func TestSourceMap(t *testing.T) {
	t.Setenv("PATCH_0", `{"op":"add","path":"/grand/value5","value":"envpatch"}`)
	t.Setenv("V1", "99")
	fs := newMockFS(map[string][]byte{
		"test.yml": []byte(`
_directives:
  include:
    - parent.yml
  patches:
    - op: add
      path: /test/value
      value: ooo
test:
  value: 10
  c1: ${V1:98}
  value2:
    - 20
`),
		"parent.yml": []byte(`
_directives:
  include:
    - grand-parent.yml
    - grand-parent2.yml
  patches:
    - op: add
      path: /test/value2/1
      value: 99
parent:
  c2: 100
  value3:
    - 30
    - 40
test:
  c1: 999
  value2:
    - 40
`),
		"grand-parent.yml": []byte(`
grand:
  value4: aaa
parent:
  c3: 777
  value3:
    - 30
    - 60
test:
  value2:
    - 70
`),
		"grand-parent2.yml": []byte(`
grand:
  value4: bbb
  value5: ccc
`),
	})
	var result yaml.Node
	err := Load("test.yml", &result, WithFileSystem(fs),
		WithSourceMapComment(), WithSourceMapKey("_sourcemap"), WithEnvJSONPatches("PATCH"))
	if err != nil {
		t.Error(err.Error())
	}
	bs, err := yaml.Marshal(&result)
	if err != nil {
		t.Error(err.Error())
	}
	expected := `_sourcemap:
    sources:
        - test.yml
        - grand-parent.yml
        - grand-parent2.yml
        - ${PATCH_0}
        - parent.yml
    mappings:
        - path: /
          file: test.yml
          line: 2
        - path: /grand
          file: grand-parent.yml
          line: 2
        - path: /grand/value4
          file: grand-parent2.yml
          line: 3
        - path: /grand/value5
          file: ${PATCH_0}
          line: 1
        - path: /parent
          file: grand-parent.yml
          line: 4
        - path: /parent/c2
          file: parent.yml
          line: 11
        - path: /parent/c3
          file: grand-parent.yml
          line: 5
        - path: /parent/value3
          file: grand-parent.yml
          line: 6
        - path: /parent/value3/0
          file: grand-parent.yml
          line: 7
        - path: /parent/value3/1
          file: grand-parent.yml
          line: 8
        - path: /parent/value3/2
          file: parent.yml
          line: 13
        - path: /parent/value3/3
          file: parent.yml
          line: 14
        - path: /test
          file: grand-parent.yml
          line: 9
        - path: /test/c1
          file: test.yml
          line: 11
        - path: /test/value
          file: test.yml
          line: 8
        - path: /test/value2
          file: grand-parent.yml
          line: 10
        - path: /test/value2/0
          file: grand-parent.yml
          line: 11
        - path: /test/value2/1
          file: parent.yml
          line: 9
        - path: /test/value2/2
          file: parent.yml
          line: 18
        - path: /test/value2/3
          file: test.yml
          line: 13
grand: #  grand-parent.yml:2
    value4: bbb #  grand-parent2.yml:3
    value5: envpatch #  ${PATCH_0}:1
parent: #  grand-parent.yml:4
    c2: 100 #  parent.yml:11
    c3: 777 #  grand-parent.yml:5
    value3: #  grand-parent.yml:6
        - 30 #  grand-parent.yml:7
        - 60 #  grand-parent.yml:8
        - 30 #  parent.yml:13
        - 40 #  parent.yml:14
test: #  grand-parent.yml:9
    c1: 99 #  test.yml:11
    value: ooo #  test.yml:8
    value2: #  grand-parent.yml:10
        - 70 #  grand-parent.yml:11
        - 99 #  parent.yml:9
        - 40 #  parent.yml:18
        - 20 #  test.yml:13
`
	if expected != string(bs) {
		t.Errorf("sourcemap has some problems: \n%s", string(bs))
	}
}
