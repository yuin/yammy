package yammy_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	. "github.com/yuin/yammy"
)

func TestPatchReplace(t *testing.T) {
	fs := newMockFS(map[string][]byte{
		"test.yml": []byte(`
_directives:
  patches:
    - op: replace
      path: /test/value
      value: aaa
    - op: replace
      path: /test/value2/1
      value: bbb
test:
  value: 10
  value2:
    - "000"
    - "111"
    - "222"
`),
	})

	var result map[any]any
	err := Load("test.yml", &result, WithFileSystem(fs))
	if err != nil {
		t.Error(err.Error())
	}

	if "aaa" != result["test"].(map[string]any)["value"] {
		t.Error("failed to patch variables")
	}
	if "bbb" != result["test"].(map[string]any)["value2"].([]any)[1] {
		t.Error("failed to patch variables")
	}
}

func TestPatchRemoveError(t *testing.T) {
	cases := []struct {
		name  string
		index string
		err   string
	}{
		{
			name:  "removeOutOfBounds",
			index: "path: /test/value/10",
			err: "directive error: test.yml(line:4): can not remove a value " +
				"at index 10(path: /test/value, size: 3): yammy error",
		},
		{
			name:  "removeDoesNotExist",
			index: "path: /test/value2",
			err: "directive error: test.yml(line:4): can not remove a value " +
				"/test/value2(value does not exist): yammy error",
		},
		{
			name:  "removeScalar",
			index: "path: /test/value3/aaa",
			err: "directive error: test.yml(line:4): can not perform a remove " +
				"operation on (an) scalar node(path: /test/value3): yammy error",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			fs := newMockFS(map[string][]byte{
				"test.yml": []byte(fmt.Sprintf(`
_directives:
  patches:
    - op: remove
      %s
test:
  value:
    - "000"
    - "111"
    - "222"
  value3: 10
`, tt.index)),
			})

			var result map[any]any
			err := Load("test.yml", &result, WithFileSystem(fs))
			if err == nil || err.Error() != tt.err {
				t.Errorf("err should be '%s', but got %v", tt.err, err)
			}
		})
	}
}

func TestPatchErrorInvalid(t *testing.T) {
	fs := newMockFS(map[string][]byte{
		"test.yml": []byte(`
_directives:
  patches:
    - insertAfter: 
      - aaa
test:
  value:
    - 10
    - 20
    - 30
`),
	})

	var result map[any]any
	err := Load("test.yml", &result, WithFileSystem(fs))
	if !strings.HasPrefix(err.Error(), "directive error: test.yml(line:4): invalid patch(op and path are required)") {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestPatchErrorAddWithNonArrayNodes(t *testing.T) {
	fs := newMockFS(map[string][]byte{
		"test.yml": []byte(`
_directives:
  patches:
    - op: add
      path: /test/value/1
      value: aaa
test:
  value: aaa
`),
	})

	var result map[any]any
	err := Load("test.yml", &result, WithFileSystem(fs))
	if "directive error: test.yml(line:4): can not perform an add '1' key operation on "+
		"(an) scalar node(path: /test/value): yammy error" != err.Error() {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestPatchAdd(t *testing.T) {
	cases := []struct {
		name     string
		index    string
		expected []any
		err      string
	}{
		{
			name:     "add",
			index:    "path: /test/value/1",
			expected: []any{"000", "aaa", "111", "222"},
		},
		{
			name:     "addLast",
			index:    "path: /test/value/-",
			expected: []any{"000", "111", "222", "aaa"},
		},
		{
			name:  "addOutOfRange",
			index: "path: /test/value/10",
			err: "directive error: test.yml(line:4): can not perform an add value " +
				"before index 10(path: /test/value, size: 3): yammy error",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			fs := newMockFS(map[string][]byte{
				"test.yml": []byte(fmt.Sprintf(`
_directives:
  patches:
    - op: add
      %s
      value: aaa
test:
  value:
    - "000"
    - "111"
    - "222"
`, tt.index)),
			})

			var result map[any]any
			err := Load("test.yml", &result, WithFileSystem(fs))
			if len(tt.err) != 0 {
				if err == nil || err.Error() != tt.err {
					t.Errorf("err should be '%s', but got %v", tt.err, err)
				}
				return
			}

			if err != nil {
				t.Error(err.Error())
			}

			value := result["test"].(map[string]any)["value"].([]any)
			if !reflect.DeepEqual(tt.expected, value) {
				t.Errorf("failed to patch variables\n  expected: %#v\n  "+
					"actual: %#v\n", tt.expected, value)
			}

		})
	}
}

func TestPatchAddAutoCreate(t *testing.T) {
	fs := newMockFS(map[string][]byte{
		"test.yml": []byte(`
_directives:
  patches:
    - op: add
      path: /root/new/value
      value: aaa
test:
  value:
    - "000"
`)})

	var result map[any]any
	err := Load("test.yml", &result, WithFileSystem(fs))
	if err != nil {
		t.Fatal(err)
	}
	if "aaa" != result["root"].(map[string]any)["new"].(map[string]any)["value"] {
		t.Error("failed to add variables(w/ autocreate parent object)")
	}

	fs = newMockFS(map[string][]byte{
		"test.yml": []byte(`
_directives:
  patches:
    - op: add
      path: /root/new/-
      value: aaa
test:
  value:
    - "000"
`)})

	err = Load("test.yml", &result, WithFileSystem(fs))
	if err != nil {
		t.Fatal(err)
	}
	if "aaa" != result["root"].(map[string]any)["new"].([]any)[0] {
		t.Error("failed to add variables(w/ autocreate parent sequence)")
	}
}

func TestPatchLoadOption(t *testing.T) {
	fs := newMockFS(map[string][]byte{
		"test.yml": []byte(`
test:
  value: 10
  value2:
    - "000"
    - "111"
    - "222"
`),
	})
	t.Setenv("PATCH_0", `{
		"op":    "replace",
		"path":  "/test/value",
		"value": "aaa"
	} `)
	t.Setenv("PATCH_1", ` {
		"op":    "replace",
		"path":  "/test/value2/1",
		"value": "bbb"
	} `)

	var result map[any]any
	err := Load("test.yml", &result, WithFileSystem(fs), WithEnvJSONPatches("PATCH"))
	if err != nil {
		t.Fatal(err)
	}

	if "aaa" != result["test"].(map[string]any)["value"] {
		t.Error("failed to patch variables")
	}
	if "bbb" != result["test"].(map[string]any)["value2"].([]any)[1] {
		t.Error("failed to patch variables")
	}
}
