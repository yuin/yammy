// Package yammy makes YAML/JSON composable.
package yammy

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/bmatcuk/doublestar/v4"
	"gopkg.in/yaml.v3"
)

type loadConfig struct {
	FS               fs.FS
	DirectiveKey     string
	SourceMapKey     string
	SourceMapComment bool
	VarResolver      VarResolver
}

// LoadOption is an option for [Load] .
type LoadOption func(*loadConfig)

// WithFileSystem is an option that specifies a file system.
// This defaults to [os.DirFS](".") .
func WithFileSystem(v fs.FS) LoadOption {
	return func(c *loadConfig) {
		c.FS = v
	}
}

// WithDirectiveKey is an option that specifies a directive key.
// This defaults to '_directives'.
func WithDirectiveKey(v string) LoadOption {
	return func(c *loadConfig) {
		c.DirectiveKey = v
	}
}

// WithSourceMapKey is an option that specifies a source map key.
// This defaults to an empty string(that means source map is disabled) .
func WithSourceMapKey(v string) LoadOption {
	return func(c *loadConfig) {
		c.SourceMapKey = v
	}
}

// WithSourceMapComment is an option that enables source map comments.
func WithSourceMapComment() LoadOption {
	return func(c *loadConfig) {
		c.SourceMapComment = true
	}
}

// WithVarResolver is an option that set a resolver for variables.
// This defaults to a resolver that resolve variables as follows:
//
//  1. if a variable is defined as an environment variable, returns it.
//  2. if a variable is defined in yammy directives, returns it.
//  3. other wise, returns [ErrVarNotFound] .
func WithVarResolver(v VarResolver) LoadOption {
	return func(c *loadConfig) {
		c.VarResolver = v
	}
}

// Load loads given YAML/JON file.
func Load(name string, dest any, opts ...LoadOption) error {
	c := &loadConfig{
		FS:               os.DirFS("."),
		DirectiveKey:     "_directives",
		SourceMapKey:     "",
		SourceMapComment: false,
		VarResolver:      nil,
	}
	for _, opt := range opts {
		opt(c)
	}

	vNode := &yaml.Node{}
	vNode.Kind = yaml.MappingNode
	variables := newNode(vNode, name)
	nd, err := loadNode(name, c, variables)
	if err != nil {
		return err
	}
	nd.File = name

	varResolver := c.VarResolver
	if varResolver == nil {
		varResolver = newDefaultVarResolver(variables)
	}

	err = processVars(nd, varResolver)
	if err != nil {
		return err
	}

	if c.SourceMapComment {
		nd.AddSourceComments()
	}

	if len(c.SourceMapKey) != 0 {
		sm := nd.ToSourceMap("/")
		var smNode yaml.Node
		bs, _ := yaml.Marshal(sm)
		_ = yaml.Unmarshal(bs, &smNode)
		keyNode := newStringNode(c.SourceMapKey, nd.File)
		nd.Put(keyNode, newNode(mustRootNode(&smNode), ""))
	}

	if dest != nil {
		err := nd.Decode(dest)
		if err != nil {
			return ErrYAML.New("%s: failed to map to given object", err, name)
		}
	}

	return nil
}

func loadNode(path string, c *loadConfig, allVariables *node) (*node, error) {
	fp, err := c.FS.Open(path)
	if err != nil {
		return nil, ErrIO.New("%s: failed to load given file", err, path)
	}
	bs, err := io.ReadAll(fp)
	if err != nil {
		return nil, ErrIO.New("%s: failed to load given file", err, path)
	}

	var docv yaml.Node
	doc := &docv
	err = yaml.Unmarshal(bs, doc)
	if err != nil {
		return nil, ErrYAML.New("%s: failed to parse given YAML file", err, path)
	}

	rootNode := mustRootNode(doc)
	root := newNode(rootNode, path)
	if rootNode.Kind != yaml.MappingNode {
		return nil, ErrYAML.New("%s: root node must be a mapping node(%s)", nil, path, root.KindString())
	}
	directives := root.Get(c.DirectiveKey)
	var includes, patches, variables *node
	if directives != nil {
		root.Delete(c.DirectiveKey)
		if directives.Kind != yaml.MappingNode {
			return nil, ErrYAML.New("%s: %s must be a mapping node", nil, path, c.DirectiveKey)
		}
		includes = directives.Get("include")
		patches = directives.Get("patches")
		variables = directives.Get("variables")
	}

	var files []string
	if includes != nil {
		for _, includeNode := range includes.Content {
			include := includeNode.Value
			fullPath := include
			if !filepath.IsAbs(fullPath) {
				fullPath = filepath.Join(filepath.Dir(path), include)
			}
			paths, err := doublestar.Glob(c.FS, fullPath)
			if err != nil {
				return nil, ErrIO.New("%s: failed to find a included file %s", err, path, include)
			}
			if len(paths) == 0 {
				return nil, ErrIO.New("%s: failed to find a included file %s", nil, path, include)
			}
			files = append(files, paths...)
		}
	}

	var mergedNode *node

	for _, file := range files {
		include, err := loadNode(file, c, allVariables)
		if err != nil {
			return nil, err
		}
		if mergedNode == nil {
			mergedNode = include
			continue
		}
		mergedNode, err = mergedNode.Merge(include)
		if err != nil {
			return nil, err
		}
	}
	if mergedNode == nil {
		mergedNode = root
	} else {
		mergedNode, err = mergedNode.Merge(root)
	}
	if err != nil {
		return nil, err
	}

	err = processPatchNodes(mergedNode, patches)
	if err != nil {
		return nil, err
	}

	if variables != nil {
		allVariables.Merge(variables)
	}

	return mergedNode, nil
}

func processPatchNodes(n *node, patchNodes *node) error {
	if patchNodes == nil {
		return nil
	}

	for _, patchNode := range patchNodes.Content {
		if patchNode.Kind != yaml.MappingNode {
			return ErrDirective.New("%s: invalid patch", nil, patchNode.Where())
		}
		pn := patchNode.Get("path")
		on := patchNode.Get("op")
		if pn == nil || on == nil {
			return ErrDirective.New(
				"%s: invalid patch(op and path are required)", nil, patchNode.Where())
		}
		path := pn.Value
		op := on.Value
		jp, err := parseJSONPointer(path)
		if err != nil {
			return ErrDirective.New("%s: %s", nil, patchNode.Where(), err.Error())
		}

		newValue := patchNode.Get("value")
		if op == "add" {
			if err := jsonPatchAdd(n, jp, newValue); err != nil {
				return ErrDirective.New("%s: %s", nil, patchNode.Where(), err.Error())
			}
			continue
		}
		if op == "remove" {
			if err := jsonPatchRemove(n, jp); err != nil {
				return ErrDirective.New("%s: %s", nil, patchNode.Where(), err.Error())
			}
			continue
		}
		if op == "replace" {
			_ = jsonPatchRemove(n, jp)
			if err := jsonPatchAdd(n, jp, newValue); err != nil {
				return ErrDirective.New("%s: %s", nil, patchNode.Where(), err.Error())
			}
			continue
		}

		return ErrDirective.New(
			"%s: unsupported patch operation: %s", nil, patchNode.Where(), op)
	}

	return nil
}

func jsonPatchAdd(n *node, jp jsonPointer, newValue *node) error {
	parent, child := jp.Pop()
	target, err := n.FindNodeByJSONPointer(parent.String())
	if err != nil {
		return err
	}
	if child.IsIndex && target.Kind == yaml.SequenceNode {
		i := child.Index
		if i < 0 {
			target.Content = append(target.Content, newValue)
			return nil
		}
		if i > len(target.Content) {
			return fmt.Errorf("can not perform an add value before index %d(path: %s, size: %d)",
				child.Index, parent.String(), len(target.Content))
		}
		target.Content = append(target.Content[:i+1], target.Content[i:]...)
		target.Content[i] = newValue
		return nil
	}
	if target.Kind == yaml.MappingNode {
		key := newStringNode(child.String, n.File)
		target.Put(key, newValue)
		return nil
	}
	return fmt.Errorf("can not perform an add '%s' key operation on (an) %s node(path: %s)",
		child.Original, target.KindString(), parent.String())
}

func jsonPatchRemove(n *node, jp jsonPointer) error {
	parent, child := jp.Pop()
	target, err := n.FindNodeByJSONPointer(parent.String())
	if err != nil {
		return err
	}
	if child.IsIndex && !child.IsLastIndex() && target.Kind == yaml.SequenceNode {
		i := child.Index
		if i >= len(target.Content) {
			return fmt.Errorf("can not remove a value at index %d(path: %s, size: %d)",
				child.Index, parent.String(), len(target.Content))
		}
		target.Content = append(target.Content[:i], target.Content[i+1:]...)
		return nil
	}
	if target.Kind == yaml.MappingNode {
		removed := target.Delete(child.String)
		if removed == nil {
			return fmt.Errorf("can not remove a value %s(value does not exist)", jp.String())
		}
		return nil
	}
	return fmt.Errorf("can not perform a remove operation on (an) %s node(path: %s)",
		target.KindString(), parent.String())
}

func processVars(n *node, resolver VarResolver) error {

	switch n.Kind {
	case yaml.MappingNode:
		return n.ForEachMap(func(k, v *node) error {
			return processVars(v, resolver)
		})
	case yaml.SequenceNode:
		return n.ForEachSeq(func(i int, v *node) error {
			return processVars(v, resolver)
		})
	case yaml.ScalarNode:
		if n.Tag == "!!str" {
			newString, err := expandVar(n.Value, resolver)
			if err != nil {
				return err
			}
			if n.Value != newString {
				var d yaml.Node
				err := yaml.Unmarshal([]byte(newString), &d)
				if err != nil {
					return ErrYAML.New("%s: failed to parse a variable", err, n.Where())
				}
				nd := mustRootNode(&d)
				n.SetString(nd.Value)
				n.Tag = nd.Tag
			}
		}
	}
	return nil
}
