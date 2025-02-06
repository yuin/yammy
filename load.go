// Package yammy makes YAML/JSON composable.
package yammy

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type loadConfig struct {
	FS                   fs.FS
	DirectiveKey         string
	SourceMapKey         string
	SourceMapComment     bool
	VarResolver          VarResolver
	KeepsVariables       bool
	RemovesBlockComments bool
	JSONPatches          []map[string]any
}

// LoadOption is an option for [Load] .
type LoadOption func(*loadConfig)

// WithFileSystem is an option that specifies a file system.
// If this is not specified, files will be loaded from the real file system.
// Since [fs.FS] does not allow paths start with . and .., WithFileSystem you must
// specify paths that relative to the [fs.FS] root.
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

// WithKeepsVariables is an option that keeps variables expression.
func WithKeepsVariables() LoadOption {
	return func(c *loadConfig) {
		c.KeepsVariables = true
	}
}

// WithRemovesBlockComments is an option that removes block comments.
func WithRemovesBlockComments() LoadOption {
	return func(c *loadConfig) {
		c.RemovesBlockComments = true
	}
}

// WithJSONPatches is an option that specifies JSON patches.
// Arguments must be a slice of JSON patches.
// In addition to the standard JSON patch properties(op, path, value),
// 'source' property is also supported.
// 'source' will be used as a source map.
func WithJSONPatches(v []map[string]any) LoadOption {
	return func(c *loadConfig) {
		c.JSONPatches = v
	}
}

// WithEnvJSONPatches is an option that specifies JSON patches from environment variables.
// WithEnvJSONPatches sorts environment variables by the name and parses them as JSON patches.
//
// i.e.: WithEnvJSONPatches("PATCH")
//
// - PATCH_0='{"op":"add","path":"/test","value":"aaa"}'
// - PATCH_1='{"op":"replace","path":"/test","value":"bbb"}'
//
// WithEnvJSONPatches panics if failed to parse environment variables as JSON.
func WithEnvJSONPatches(prefix string) LoadOption {
	patches, err := envJSONPatches(prefix)
	if err != nil {
		panic(err)
	}
	return WithJSONPatches(patches)
}

// Load loads given YAML/JON file.
func Load(name string, dest any, opts ...LoadOption) error {
	c := &loadConfig{
		FS:                   nil,
		DirectiveKey:         "_directives",
		SourceMapKey:         "",
		SourceMapComment:     false,
		VarResolver:          nil,
		KeepsVariables:       false,
		RemovesBlockComments: false,
	}
	for _, opt := range opts {
		opt(c)
	}

	vNode := &yaml.Node{}
	vNode.Kind = yaml.MappingNode
	variables := newNode(vNode, name, c.RemovesBlockComments)
	nd, err := loadNode(name, c, variables)
	if err != nil {
		return err
	}
	nd.File = name

	varResolver := c.VarResolver
	if varResolver == nil {
		varResolver = newCompositeVarResolver(
			envVarResolver,
			newDirectiveVarResolver(variables))
	} else {
		varResolver = newCompositeVarResolver(
			varResolver,
			newDirectiveVarResolver(variables))
	}

	err = processVars(nd, varResolver, c.KeepsVariables)
	if err != nil {
		return err
	}

	if len(c.JSONPatches) != 0 {
		var pNode yaml.Node
		bs, _ := yaml.Marshal(c.JSONPatches)
		_ = yaml.Unmarshal(bs, &pNode)
		pnr := newNode(pNode.Content[0],
			"<LoadOption.JSONPatches>", c.RemovesBlockComments)
		pnr.ClearPosition(true)
		for _, pn := range pnr.Content {
			if source := pn.Get("source"); source != nil {
				pn = newNode(pn.Node, source.Value, c.RemovesBlockComments)
			}
			err := processPatchNode(nd, pn)
			if err != nil {
				return err
			}
		}
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
		nd.Put(keyNode, newNode(mustRootNode(&smNode), "", c.RemovesBlockComments))
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
	fp, err := fsOpen(c.FS, path)
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
	root := newNode(rootNode, path, c.RemovesBlockComments)
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
			paths, err := fsGlob(c.FS, fullPath)
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
		_, err = allVariables.Merge(variables)
		if err != nil {
			return nil, err
		}
	}

	return mergedNode, nil
}

func processPatchNodes(n *node, patchNodes *node) error {
	if patchNodes == nil {
		return nil
	}

	for _, patchNode := range patchNodes.Content {
		err := processPatchNode(n, patchNode)
		if err != nil {
			return err
		}
	}

	return nil
}

func processPatchNode(n *node, patchNode *node) error {
	if patchNode == nil {
		return nil
	}

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
		return nil
	}
	if op == "remove" {
		if err := jsonPatchRemove(n, jp); err != nil {
			return ErrDirective.New("%s: %s", nil, patchNode.Where(), err.Error())
		}
		return nil
	}
	if op == "replace" {
		_ = jsonPatchRemove(n, jp)
		if err := jsonPatchAdd(n, jp, newValue); err != nil && !errors.Is(err, errNotFound) {
			return ErrDirective.New("%s: %s", nil, patchNode.Where(), err.Error())
		}
		return nil
	}

	return ErrDirective.New(
		"%s: unsupported patch operation: %s", nil, patchNode.Where(), op)
}

func jsonPatchAdd(n *node, jp jsonPointer, newValue *node) error {
	parent, child := jp.Pop()
	target, err := n.FindNodeByJSONPointer(parent.String())

	if errors.Is(err, errNotFound) {
		err = ensureJSONPointerParent(n, jp)
		if err != nil {
			return err
		}
		target, err = n.FindNodeByJSONPointer(parent.String())
	}
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

func ensureJSONPointerParent(obj *node, pointer jsonPointer) error {
	parent := obj
	for i := 0; i < len(pointer)-1; i++ {
		t := pointer[i]
		n := pointer[i+1]
		if t.IsIndex {
			if parent.Kind == yaml.SequenceNode {
				if len(parent.Content) <= t.Index {
					if parent.Content[t.Index] == nil {
						if n.IsIndex {
							parent.Content[t.Index] = newSequenceNode(parent.File)
						} else {
							parent.Content[t.Index] = newMappingNode(parent.File)
						}
					}
					parent = parent.Content[t.Index]
					continue
				}
			}
		}
		if parent.Kind == yaml.MappingNode {
			key := newStringNode(t.String, parent.File)
			if v := parent.Get(t.String); v == nil {
				if n.IsIndex {
					parent.Put(key, newSequenceNode(parent.File))
				} else {
					parent.Put(key, newMappingNode(parent.File))
				}
			}
			parent = parent.Get(t.String)
			continue
		}
		return fmt.Errorf("can not evaluate an index %s on %T object", t.String, parent)
	}
	return nil
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

func processVars(n *node, resolver VarResolver, keepsVariables bool) error {

	switch n.Kind {
	case yaml.MappingNode:
		return n.ForEachMap(func(_, v *node) error {
			return processVars(v, resolver, keepsVariables)
		})
	case yaml.SequenceNode:
		return n.ForEachSeq(func(_ int, v *node) error {
			return processVars(v, resolver, keepsVariables)
		})
	case yaml.ScalarNode:
		if n.Tag == "!!str" {
			newString, err := expandVar(n.Value, resolver, keepsVariables)
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

func envJSONPatches(prefix string) ([]map[string]any, error) {
	patches := map[string]string{}
	var keys []string
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, prefix) {
			continue
		}
		parts := strings.SplitN(env, "=", 2)
		patches[parts[0]] = parts[1]
		keys = append(keys, parts[0])
	}
	sort.Strings(keys)

	var result []map[string]any
	for _, k := range keys {
		p := patches[k]
		var patch map[string]any
		err := json.Unmarshal([]byte(p), &patch)
		if err != nil {
			return nil, ErrYAML.New("failed to parse %s environment variable JSON patch", err, p)
		}
		patch["source"] = "${" + k + "}"
		result = append(result, patch)
	}

	return result, nil
}
