package yammy_test

import (
	"errors"
	"testing"

	. "github.com/yuin/yammy"
)

func TestStringVar(t *testing.T) {

	fs := newMockFS(map[string][]byte{
		"test.yml": []byte(`
test:
  value: ${KEY:10} bbb ${KEY2}
  value2:
    - ${KEY2:aaa}
  value3: ${AAA:"e \"e}\\ e"} 
  value4: ddd ${AAA:"e e e"} fff
`),
	})
	var result map[any]any
	t.Setenv("KEY", "aaa")
	t.Setenv("KEY2", "ccc")
	err := Load("test.yml", &result, WithFileSystem(fs))
	if err != nil {
		t.Error(err.Error())
	}

	if "aaa bbb ccc" != result["test"].(map[string]any)["value"] {
		t.Error("failed to evaluate variables")
	}
	if "ccc" != result["test"].(map[string]any)["value2"].([]any)[0] {
		t.Error("failed to evaluate variables")
	}
	if `e "e}\ e` != result["test"].(map[string]any)["value3"] {
		println(result["test"].(map[string]any)["value3"].(string))
		t.Error("failed to evaluate variables")
	}
	if "ddd e e e fff" != result["test"].(map[string]any)["value4"] {
		t.Error("failed to evaluate variables")
	}

	result = map[any]any{}
	err = Load("test.yml", &result, WithFileSystem(fs), WithKeepsVariables())
	if err != nil {
		t.Error(err.Error())
	}

	if "${KEY:aaa} bbb ${KEY2:ccc}" != result["test"].(map[string]any)["value"] {
		t.Error("failed to evaluate variables")
	}
	if "${KEY2:ccc}" != result["test"].(map[string]any)["value2"].([]any)[0] {
		t.Error("failed to evaluate variables")
	}
	if `${AAA:"e \"e}\\ e"}` != result["test"].(map[string]any)["value3"] {
		t.Error("failed to evaluate variables")
	}
	if `ddd ${AAA:"e e e"} fff` != result["test"].(map[string]any)["value4"] {
		t.Error("failed to evaluate variables")
	}
}

func TestDefaultIntVar(t *testing.T) {
	fs := newMockFS(map[string][]byte{
		"test.yml": []byte(`
test:
  value: ${KEY:10}
`),
	})
	var result map[any]any
	err := Load("test.yml", &result, WithFileSystem(fs))
	if err != nil {
		t.Error(err.Error())
	}

	if 10 != result["test"].(map[string]any)["value"] {
		t.Error("failed to evaluate variables")
	}

}

func TestDefaultStringVar(t *testing.T) {
	fs := newMockFS(map[string][]byte{
		"test.yml": []byte(`
test:
  value: ${KEY:"a\"\\na}"}
`),
	})
	var result map[any]any
	err := Load("test.yml", &result, WithFileSystem(fs))
	if err != nil {
		t.Error(err.Error())
	}

	if "a\"\\na}" != result["test"].(map[string]any)["value"] {
		t.Error("failed to evaluate variables")
	}
}

func TestVarNotFound(t *testing.T) {
	fs := newMockFS(map[string][]byte{
		"test.yml": []byte(`
test:
  value: ${KEY}
`),
	})
	var result map[any]any
	err := Load("test.yml", &result, WithFileSystem(fs))
	if err == nil || "variable not found: KEY not found: yammy error" != err.Error() {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestVarDirective(t *testing.T) {
	fs := newMockFS(map[string][]byte{
		"test.yml": []byte(`
_directives:
  variables:
    KEY: 10
    KEY2: bbb
test:
  value: ${KEY:0}
  value2: ${KEY2}
`),
	})
	var result map[any]any
	t.Setenv("KEY2", "bbb")
	err := Load("test.yml", &result, WithFileSystem(fs))
	if err != nil {
		t.Error(err.Error())
	}

	if 10 != result["test"].(map[string]any)["value"] {
		t.Error("failed to evaluate variables")
	}
	if "bbb" != result["test"].(map[string]any)["value2"] {
		t.Error("failed to evaluate variables")
	}

}

func TestInvalidVar(t *testing.T) {
	fs := newMockFS(map[string][]byte{
		"test.yml": []byte(`
test:
  value2: ${aaa
  value3: ${KEY:"aaaaa}
  value4: ${KEY:10 bbb
  value5: ${[]}
`),
	})
	var result map[any]any
	err := Load("test.yml", &result, WithFileSystem(fs))
	if err != nil {
		t.Error(err.Error())
	}

	if "${aaa" != result["test"].(map[string]any)["value2"] {
		t.Error("failed to evaluate variables")
	}
	if "${KEY:\"aaaaa}" != result["test"].(map[string]any)["value3"] {
		t.Error("failed to evaluate variables")
	}
	if "${KEY:10 bbb" != result["test"].(map[string]any)["value4"] {
		t.Error("failed to evaluate variables")
	}
	if "${[]}" != result["test"].(map[string]any)["value5"] {
		t.Error("failed to evaluate variables")
	}
}

func TestEmptyVar(t *testing.T) {
	fs := newMockFS(map[string][]byte{
		"test.yml": []byte(`
test:
  value: ${}
`),
	})
	var result map[any]any
	err := Load("test.yml", &result, WithFileSystem(fs))
	if err != nil {
		t.Error(err.Error())
	}

	if "${}" != result["test"].(map[string]any)["value"] {
		t.Error("failed to evaluate variables")
	}

}

func TestForceStringVar(t *testing.T) {
	fs := newMockFS(map[string][]byte{
		"test.yml": []byte(`
test:
  value: ${KEY:"10.0"}
`),
	})
	var result map[any]any
	err := Load("test.yml", &result, WithFileSystem(fs))
	if err != nil {
		t.Error(err.Error())
	}

	if "10.0" != result["test"].(map[string]any)["value"] {
		t.Error("failed to evaluate variables")
	}

}

func TestInvalidVarValue(t *testing.T) {
	fs := newMockFS(map[string][]byte{
		"test.yml": []byte(`
test:
  value: ${KEY:[}
`),
	})
	var result map[any]any
	err := Load("test.yml", &result, WithFileSystem(fs))
	if "yaml error: test.yml(line:3): failed to parse a variable: yaml: line 1: did not "+
		"find expected node content: yammy error" != err.Error() {
		t.Errorf("unexpected error message: %s", err.Error())
	}
	if "yaml: line 1: did not find expected node content" != errors.Unwrap(err).Error() {
		t.Errorf("unexpected error message: %s", errors.Unwrap(err).Error())
	}
}

func TestEscapedVar(t *testing.T) {
	fs := newMockFS(map[string][]byte{
		"test.yml": []byte(`
test:
  value: $${KEY:aaa}
`),
	})
	var result map[any]any
	err := Load("test.yml", &result, WithFileSystem(fs))
	if err != nil {
		t.Error(err.Error())
	}

	if "$${KEY:aaa}" != result["test"].(map[string]any)["value"] {
		t.Error("failed to evaluate variables")
	}
}

func TestVarInheritance(t *testing.T) {
	fs := newMockFS(map[string][]byte{
		"test.yml": []byte(`
_directives:
  include:
    - child.yml
  variables:
    vname: root
`),
		"child.yml": []byte(`
_directives:
  include:
    - grand-child.yml
  variables:
    vname: child
    vname2: c
vvalue2: ${vname2}
`),
		"grand-child.yml": []byte(`
vvalue: ${vname}

`),
	})
	var result map[any]any
	err := Load("test.yml", &result, WithFileSystem(fs))
	if err != nil {
		t.Error(err.Error())
	}

	if "root" != result["vvalue"] {
		t.Error("failed to evaluate variables")
	}
	if "c" != result["vvalue2"] {
		t.Error("failed to evaluate variables")
	}
}
