package yammy

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"gopkg.in/yaml.v3"
)

func mustRootNode(nd *yaml.Node) *yaml.Node {
	if nd.Kind == 0 {
		m := &yaml.Node{}
		m.Kind = yaml.MappingNode
		return m
	}
	if nd.Kind == yaml.DocumentNode {
		if len(nd.Content) < 1 {
			return nd
		}
		return nd.Content[0]
	}
	kind := "unknown"
	switch nd.Kind {
	case yaml.AliasNode:
		kind = "alias"
	case yaml.MappingNode:
		kind = "mapping"
	case yaml.ScalarNode:
		kind = "scalar"
	case yaml.SequenceNode:
		kind = "sequence"
	}
	panic("must be a document node, but got " + kind)
}

func mustJSONPointer(base string, tokens ...any) string {
	if len(tokens) == 0 {
		return base
	}
	parts := append(append([]any{}, base), tokens...)
	buf := make([]string, 0, len(parts)*2)
	for i, part := range parts {
		if s, ok := part.(string); ok {
			if len(s) == 0 {
				continue
			}
			if len(buf) != 0 {
				buf = append(buf, "/")
			}
			if i != 0 {
				buf = append(buf, strings.ReplaceAll(
					strings.ReplaceAll(s, "~", "~0"), "/", "~1"))
			} else {
				buf = append(buf, s)
			}
			continue
		}
		if i, ok := part.(int); ok {
			buf = append(buf, fmt.Sprintf("/%d", i))
			continue
		}
		panic(fmt.Errorf("uknown path parts: %v", part))
	}
	ret := strings.Join(buf, "")
	if "//" == ret[0:2] {
		return ret[1:]
	}
	return ret
}

func isVarName(c byte, i int) bool {
	if ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') {
		return true
	}
	if i != 0 {
		return ('0' <= c && c <= '9') || c == '_' || c == '-'
	}
	return false
}

type vvv struct {
	start int
	end   int
	name  string
	def   string
	tag   string
}

// VarResolver resolves variables named key.
// VarResolver returns [ErrVarNotFound] if variables not found.
type VarResolver func(key string) (string, error)

func newCompositeVarResolver(resolvers ...VarResolver) VarResolver {
	return func(key string) (string, error) {
		for _, r := range resolvers {
			v, err := r(key)
			if errors.Is(err, ErrVarNotFound) {
				continue
			}
			return v, err
		}
		return "", ErrVarNotFound.New("%s not found", nil, key)
	}
}

func newDirectiveVarResolver(vars *node) VarResolver {
	return func(key string) (string, error) {
		if vars != nil && vars.Kind == yaml.MappingNode {
			v := vars.Get(key)
			if v != nil {
				if v.Kind != yaml.ScalarNode {
					return "", ErrDirective.New("variable %s must be a scalar node", nil, key)
				}
				return v.Value, nil
			}
		}
		return "", ErrVarNotFound.New("%s not found", nil, key)
	}
}

func envVarResolver(key string) (string, error) {
	ev, ok := os.LookupEnv(key)
	if ok {
		return ev, nil
	}
	return "", ErrVarNotFound.New("%s not found", nil, key)
}

func expandVar(v string, resolv VarResolver, keepsVariables bool) (string, error) {
	i := 0
	state := 0
	varStarts := -1
	varName := ""
	var vars []vvv
	for ; i < len(v); i++ {
		c := v[i]
		switch state {
		case 0: // initial state
			if c == '$' {
				if i != len(v)-1 {
					c2 := v[i+1]
					if c2 == '$' {
						i++
					}
					if c2 == '{' {
						varStarts = i
						state = 1
					}
				}
			}
		case 1: // var
			state = 2
		case 2: // var name
			j := i
			for ; j < len(v) && isVarName(v[j], j); j++ { //nolint
			}
			varName = v[i:j]
			i = j - 1
			if len(varName) == 0 {
				state = 0
				continue
			}
			state = 3
		case 3: // default value or end var
			if c == ':' {
				state = 4
				continue
			}
			if c == '}' {
				vars = append(vars, vvv{
					start: varStarts,
					end:   i + 1,
					name:  varName,
					tag:   "str",
				})
				state = 0
			}
		case 4: // default value
			if c == '"' {
				value, end := parseVarString(v, i, c)
				state = 0
				if end < 0 || len(v) == end {
					continue
				}
				i = end - 1
				if v[end] == '}' {
					va := vvv{
						start: varStarts,
						end:   end + 1,
						name:  varName,
					}
					va.def = string(value)
					va.tag = "str"
					vars = append(vars, va)
				}
			} else {
				j := i
				for ; j < len(v) && v[j] != '}'; j++ { //nolint
				}
				state = 0
				if j == len(v) {
					continue
				}
				value := v[i:j]
				i = j - 1
				if v[j] == '}' {
					vars = append(vars, vvv{
						start: varStarts,
						end:   j + 1,
						name:  varName,
						def:   string(value),
						tag:   guessScalarNodeTag(value),
					})
				}
			}
		}
	}
	if len(vars) == 0 {
		return v, nil
	}

	offset := 0
	var ret []byte
	for _, vv := range vars {
		ret = append(ret, v[offset:vv.start]...)
		resolved, err := resolv(vv.name)
		if err != nil {
			resolved = vv.def
			if errors.Is(err, ErrVarNotFound) {
				if len(vv.def) == 0 && !keepsVariables {
					return "", err
				}
			} else {
				return "", err
			}
		}
		isSingleVariable := len(vars) == 1 && vv.end == len(v) && vv.start == 0
		if !keepsVariables {
			if len(resolved) == 0 && isSingleVariable {
				ret = append(ret, `""`...)
			} else if vv.tag == "str" && isSingleVariable && shouldQuote(resolved) {
				ret = append(ret, quote(resolved)...)
			} else {
				ret = append(ret, resolved...)
			}
		} else {
			if len(resolved) == 0 {
				ret = append(ret, fmt.Sprintf("${%s}", vv.name)...)
			} else if vv.tag == "str" && shouldQuote(resolved) {
				ret = append(ret, fmt.Sprintf("${%s:%s}", vv.name, quote(resolved))...)
			} else {
				ret = append(ret, fmt.Sprintf("${%s:%s}", vv.name, resolved)...)
			}
		}
		offset = vv.end
	}
	ret = append(ret, v[offset:]...)
	return string(ret), nil
}

func parseVarString(v string, i int, q byte) ([]byte, int) {
	i++ //skip `q`
	l := len(v)
	var buf []byte
	for i < l {
		c := v[i]
		if c == '\\' && i != l-1 {
			if v[i+1] == q {
				buf = append(buf, q)
				i += 2
			} else if v[i+1] == '\\' {
				buf = append(buf, '\\')
				i += 2
			} else {
				buf = append(buf, c)
				i++
			}
			continue
		}
		if c == q {
			return buf, i + 1
		}
		buf = append(buf, c)
		i++
	}
	return buf, -1
}

var space = regexp.MustCompile(`\s`)

func shouldQuote(s string) bool {
	return strings.Contains(s, "}") || strings.Contains(s, "\"") || guessScalarNodeTag(s) != "str" || space.MatchString(s)
}

func quote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return fmt.Sprintf(`"%s"`, s)
}

func guessScalarNodeTag(s string) string {
	if s == "null" {
		return "null"
	}
	if s == "true" || s == "false" || s == "yes" || s == "no" || s == "on" || s == "off" {
		return "bool"
	}
	if _, err := strconv.Atoi(s); err == nil {
		return "int"
	}
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return "float"
	}
	return "str"
}

type jsonPointerToken struct {
	String   string
	IsIndex  bool
	Index    int
	Original string
	Path     string
}

func (t *jsonPointerToken) IsLastIndex() bool {
	return t.IsIndex && t.Index < 0
}

type jsonPointer []*jsonPointerToken

func (p jsonPointer) Pop() (jsonPointer, *jsonPointerToken) {
	return jsonPointer(p[0 : len(p)-1]), p[len(p)-1]
}

func (p jsonPointer) String() string {
	s := make([]string, len(p))
	for i, t := range p {
		s[i] = t.Original
	}
	return "/" + strings.Join(s, "/")
}

func parseJSONPointer(path string) (jsonPointer, error) {
	if !strings.HasPrefix(path, "/") {
		return nil, fmt.Errorf("Invalid JSON Pointer: %s", path)
	}
	parts := strings.Split(path[1:], "/")
	ret := make([]*jsonPointerToken, len(parts))
	for i, part := range parts {
		t := &jsonPointerToken{
			Original: part,
			Path:     "/" + strings.Join(parts[:i+1], "/"),
		}
		ret[i] = t
		t.String = strings.ReplaceAll(part, "~1", "/")
		t.String = strings.ReplaceAll(t.String, "~0", "~")
		v, err := strconv.Atoi(t.String)
		if err == nil && v > -1 {
			t.Index = v
			t.IsIndex = true
		}
		if t.String == "-" {
			t.Index = -1
			t.IsIndex = true
		}
	}
	return jsonPointer(ret), nil
}

func fsOpen(f fs.FS, path string) (fs.File, error) {
	if f != nil {
		return f.Open(path)
	}
	return os.Open(path)
}

func fsGlob(f fs.FS, pattern string) ([]string, error) {
	if f != nil {
		return doublestar.Glob(f, pattern)
	}
	base, p := doublestar.SplitPattern(pattern)
	lst, err := doublestar.Glob(os.DirFS(base), p)
	if err != nil {
		return []string{}, err
	}
	for i, b := range lst {
		lst[i] = filepath.Join(base, b)
	}
	return lst, err
}
