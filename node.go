package yammy

import (
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"
)

var nbreak = errors.New("break")       // nolint
var ncontinue = errors.New("continue") // nolint

type node struct {
	*yaml.Node
	File    string
	Content []*node
}

func newStringNode(s string, file string) *node {
	key := &yaml.Node{}
	key.SetString(s)
	return newNode(key, file, false)
}

func newNode(n *yaml.Node, file string, rbc bool) *node {
	nd := &node{
		Node: n,
		File: file,
	}
	initNode(nd, rbc)
	return nd
}

func initNode(n *node, rbc bool) {
	if rbc {
		n.Node.FootComment = ""
		n.Node.HeadComment = ""
	}
	nd := n.Node
	for i := 0; i < len(nd.Content); i++ {
		elm := nd.Content[i]
		n.Content = append(n.Content, newNode(elm, n.File, rbc))
	}
}

func (n *node) Where() string {
	return fmt.Sprintf("%s(line:%d)", n.File, n.Line)
}

func (n *node) ToYAMLNode() *yaml.Node {
	nd := n.Node
	nd.Content = make([]*yaml.Node, len(n.Content))
	for i := 0; i < len(n.Content); i++ {
		nd.Content[i] = n.Content[i].ToYAMLNode()
	}
	return nd
}

func (n *node) FindNodeByYAMLNode(nd *yaml.Node) *node {
	if n.Node == nd {
		return n
	}
	for i := 0; i < len(n.Content); i++ {
		ret := n.Content[i].FindNodeByYAMLNode(nd)
		if ret != nil {
			return ret
		}
	}
	return nil
}

var errAfterLastElement = errors.New("after last element")
var errNotFound = errors.New("can not find nodes matches path")

func findNodeByJSONPointer(n *node, pointer jsonPointer, index int) (*node, error) {
	if index == len(pointer) {
		return n, nil
	}
	t := pointer[index]
	if n.Kind == yaml.MappingNode {
		if c := n.Get(t.String); c != nil {
			return findNodeByJSONPointer(c, pointer, index+1)
		}
	}
	if !t.IsIndex {
		return nil, errNotFound
	}

	if n.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("can not evaluate an index %d on %s object", t.Index, n.KindString())
	}
	if t.Index < 0 {
		return n, errAfterLastElement
	}
	if t.Index >= len(n.Content) {
		return nil, fmt.Errorf("out of bounds index %d", t.Index)
	}
	return findNodeByJSONPointer(n.Content[t.Index], pointer, index+1)
}

func (n *node) FindNodeByJSONPointer(path string) (*node, error) {
	p, err := parseJSONPointer(path)
	if err != nil {
		return nil, err
	}
	if path == "/" {
		return n, nil
	}
	ret, err := findNodeByJSONPointer(n, p, 0)
	if err != nil {
		return nil, fmt.Errorf("%s: %s", path, err.Error())
	}
	return ret, nil
}

func (n *node) KindString() string {
	switch n.Kind {
	case yaml.DocumentNode:
		return "document"
	case yaml.SequenceNode:
		return "sequence"
	case yaml.MappingNode:
		return "mapping"
	case yaml.ScalarNode:
		return "scalar"
	case yaml.AliasNode:
		return "alias"
	}
	return "unknown"
}

func (n *node) HasKey(key *node) bool {
	if n.Kind != yaml.MappingNode {
		return false
	}
	found := false
	_ = n.ForEachMap(func(k, _ *node) error {
		if key.Value == k.Value {
			found = true
			return nbreak
		}
		return nil
	})
	return found
}

func (n *node) ForEachMap(f func(k, v *node) error) error {
	for i := 0; i < len(n.Content); i += 2 {
		k := n.Content[i]
		v := n.Content[i+1]
		err := f(k, v)
		if errors.Is(err, nbreak) {
			return nil
		}
		if errors.Is(err, ncontinue) {
			continue
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (n *node) ForEachSeq(f func(i int, v *node) error) error {
	for i := 0; i < len(n.Content); i++ {
		err := f(i, n.Content[i])
		if errors.Is(err, nbreak) {
			return nil
		}
		if errors.Is(err, ncontinue) {
			continue
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (n *node) Get(key string) *node {
	if n.Kind != yaml.MappingNode {
		return nil
	}
	var ret *node
	_ = n.ForEachMap(func(k, v *node) error {
		if k.Value == key {
			ret = v
			return nbreak
		}
		return nil
	})
	return ret
}

func (n *node) Delete(key string) *node {
	if n.Kind != yaml.MappingNode {
		return nil
	}
	newContent := []*node{}
	var ret *node
	for i := 0; i < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			ret = n.Content[i+1]
			continue
		}
		newContent = append(newContent, n.Content[i], n.Content[i+1])
	}
	n.Content = newContent
	if ret == nil {
		return nil
	}
	return ret
}

func (n *node) Put(key, value *node) *node {
	if n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(n.Content); i += 2 {
		k := n.Content[i]
		v := n.Content[i+1]
		if k.Value == key.Value {
			n.Content[i+1] = value
			return v
		}
	}

	pos := len(n.Content)
	if key.Kind == yaml.ScalarNode {
		for i := 0; i < len(n.Content); i += 2 {
			if key.Value < n.Content[i].Value {
				pos = i
				break
			}
		}
	}

	n.Content = append(n.Content[:pos], append([]*node{key, value}, n.Content[pos:]...)...)
	return nil
}

func (n *node) Decode(target any) error {
	return n.ToYAMLNode().Decode(target)
}

func (n *node) Append(value *node) {
	if n.Kind != yaml.SequenceNode {
		return
	}
	n.Content = append(n.Content, value)
}

func toSourceMap(n *node, p string, sm *SourceMap) {
	sm.AddSource(n.File)
	switch n.Kind {
	case yaml.MappingNode:
		_ = n.ForEachMap(func(k, v *node) error {
			sm.AddMapping(mustJSONPointer(p, k.Value), k.File, k.Line)
			toSourceMap(v, mustJSONPointer(p, k.Value), sm)
			return nil
		})
	case yaml.SequenceNode:
		_ = n.ForEachSeq(func(i int, v *node) error {
			toSourceMap(v, mustJSONPointer(p, i), sm)
			return nil
		})
	default:
		sm.AddMapping(p, n.File, n.Line)
	}
}

func (n *node) ToSourceMap(p string) *SourceMap {
	sm := newSourceMap()
	sm.AddMapping(p, n.File, n.Line)
	toSourceMap(n, p, sm)
	return sm
}

func (n *node) AddSourceComments() {
	switch n.Kind {
	case yaml.MappingNode:
		_ = n.ForEachMap(func(k, v *node) error {
			if len(v.Anchor) == 0 {
				k.AddSourceComments()
			}
			v.AddSourceComments()
			return nil
		})
	case yaml.SequenceNode:
		_ = n.ForEachSeq(func(_ int, v *node) error {
			v.AddSourceComments()
			return nil
		})
	default:
		n.LineComment += fmt.Sprintf(" %s:%d", n.File, n.Line)
	}
}

func (n *node) Merge(other *node) (*node, error) {
	if n.Kind != other.Kind {
		return other, nil
	}
	switch n.Kind {
	case yaml.MappingNode:
		err := other.ForEachMap(func(k, v *node) error {
			if n.HasKey(k) {
				nv, err := n.Get(k.Value).Merge(v)
				if err != nil {
					return err
				}
				n.Put(k, nv)
			} else {
				n.Put(k, v)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		return n, nil
	case yaml.SequenceNode:
		_ = other.ForEachSeq(func(_ int, v *node) error {
			n.Append(v)
			return nil
		})
		return n, nil
	default:
		return other, nil
	}
}

func (n *node) ClearPosition(recursive bool) {
	n.Line = 1
	n.Column = 1
	for _, c := range n.Content {
		c.ClearPosition(recursive)
	}
}

// SourceMap is a mapping that nodes and files.
type SourceMap struct {
	// Sources is a file paths that contain nodes.
	Sources []string

	// Mappings is a mappings that nodes and files.
	Mappings []*Mapping
}

// Mapping is a mapping that nodes and files.
type Mapping struct {
	// Path is a YAML path.
	Path string

	// File is a file path.
	File string

	// Line is a line in the File.
	Line int
}

func newSourceMap() *SourceMap {
	return &SourceMap{}
}

// AddSource adds a source file to this source map.
func (s *SourceMap) AddSource(source string) {
	for _, s := range s.Sources {
		if source == s {
			return
		}
	}
	s.Sources = append(s.Sources, source)
}

// FindMap finds a mapping with path.
// FindMap returns nil if no mappings found.
func (s *SourceMap) FindMap(path string) *Mapping {
	for _, m := range s.Mappings {
		if m.Path == path {
			return m
		}
	}
	return nil
}

// AddMapping adds a new mapping to this source map.
func (s *SourceMap) AddMapping(path, file string, line int) {
	m := s.FindMap(path)
	if m != nil {
		m.File = file
		m.Line = line
		return
	}
	s.Mappings = append(s.Mappings, &Mapping{
		Path: path,
		File: file,
		Line: line,
	})
}
