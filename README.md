# yammy

[![https://pkg.go.dev/github.com/yuin/yammy](https://pkg.go.dev/badge/github.com/yuin/yammy.svg)](https://pkg.go.dev/github.com/yuin/yammy)
[![https://github.com/yuin/yammy/actions?query=workflow:test](https://github.com/yuin/yammy/workflows/test/badge.svg?branch=main&event=push)](https://github.com/yuin/yammy/actions?query=workflow:test)
[![https://goreportcard.com/report/github.com/yuin/yammy](https://goreportcard.com/badge/github.com/yuin/yammy)](https://goreportcard.com/report/github.com/yuin/yammy)

> Composable YAML/JSON made easy for CLI and Go :yum:

## Overview
yammy is a CLI tool/Go library that allows you to easily build composable YAML/JSON.
yammy aims to make configuration management made easy. 

## Features

- **Both CLI and Go library available**
- **Simple and small** : yammy does not have functionalities like script languages. So yammy library size is small.
- **Easy to debug** : yammy can save original source positions.

## Installation
### CLI
Get a binary from [releases](https://github.com/yuin/yammy/releases) .

### Go library
yammy requires Go 1.19+.

```go
$ go get -u github.com/yuin/yammy
```

## Usage
### How it works
#### Basics
yammy uses a 'directive' object that is defined in a YAML/JSON.
A directive object is defined with special key(default: `_directives`) under the root object.

```yaml
_directives: # directive object
  include:
    - base.yml
    - base2.yml
  patches:
    - path: /obj/key
      op: add
      value: 333
    - path: /arr/-
      op: add
      value: 01hoge
  variables:
    varname: value

name: value # your own data
name2: ${varname}
# .
# .
# .
```

yammy processing flow is like the following:

```python
node = load_yaml(file)
variables = load_variables(file)
json_patches = load_json_patches(file)

for included_file in include:
  included_node = load_yaml(included_file)
  included_variables  = load_variables(included_file)
  included_json_patches = load_json_patches(included_file)

  current_node = merge(current_node, included_node)
  apply_json_patches(current_node, included_json_patches)

  variables = merge(variables, included_variables)

node = merge(current_node, node)
apply_json_patches(node, json_patches)
resolve_variables(node, variables)
print(to_yaml(node))
```

#### Include files
Included nodes will be overwritten or be merged with nodes have same keys.

```
_directives:
  include:
    - base.yml

obj:
  key: overwritten
arr:
  - 10
  - 20
```

base.yml
```
obj:
  key: value
arr:
  - 0
s: string
```

results

```
obj:
  key: overwritten
arr:
  - 0
  - 10
  - 20
s: string
```


#### JSON Patch
You can define JSON Patches under the `_directives/patches` .
Available JSON Patch operations are:

- add
- replace
- remove

#### Variables
You can use variables like `${VARNAME:default value}`.

yammy resolves variable values in the following order:

- a variable that is defined in `_directives.variables`
- an enviroment variable

A default value can be quoted with `"`. Quoted values will be resolved as a string.

```
  value3: ${BBB:""}
  value4: ddd ${AAA:"e e e"} fff
```

results

```
value3: ""
value4: ddd e e e fff
```

A Variable type will be guessed by a default value. A Variable without a default value will be guessed as a string.

```
value: ${VALUE:10} # => means a scalar number node
value2: ${VALUE2:"10"} # => means a scalar string node
value3: ${VALUE3} # => no default values, means a scalar string node
value4: ${VALUE4:false} # means a scalar bool node
```

If `VALUE3=true` is set in the environment, `value3` will be `"true"`(a scalar string). 
If `VALUE4=true` is set in the environment, `value4` will be `true`(a scalar bool). 

#### Debugging
yammy can generate original node positions as a node and comments.

node:

```yaml
_sourcemap:
    sources:
        - test.yml
        - base.yml
        - root.yml
    mappings:
        - path: /
          file: test.yml
          line: 1
        - path: /anchors
          file: test.yml
          line: 11
        - path: /anchors/default
          file: test.yml
          line: 12
        - path: /anchors/default/user
          file: test.yml
          line: 13
```

comments:

```yaml
arr: #  base.yml:8
    - 0hoge #  base.yml:9
    - 1hoge #  base.yml:10
    - 2hoge #  base.yml:11
    - 3hoge #  base.yml:12
    - 01hoge #  test.yml:10
base: 10 # base comment base.yml:5
# bbb
fooo: 111 #ddd test.yml:17
hoge: value #  test.yml:15
```


### CLI
```bash
$ yammy -h
yammy [COMMAND|-h]
  COMMANDS: (default: generate)
    generate: generates a YAML/JSON file
  OPTIONS:
    -h: show this help
```

```bash
$ yammy generate -h
Usage of generate:
  -b    remove block comments(optional)
  -c    add source map comments
  -f string
        output format(yaml or json) (default "yaml")
  -h    show this help
  -i string
        source file path(required)
  -k    keep variable expressions(optional)
  -o string
        output file path(optional)
  -s string
        source map node key name
```

Examples:

test.yml

```yaml
_directives:
  include:
    - base.yml
  patches:
    - path: /obj/key
      op: add
      value: 333
    - path: /arr/-
      op: add
      value: 01hoge
anchors:
  default: &default
    user: ${USER:root}
# aaaa
hoge: value
# bbb
fooo: 111 #ddd
# ccc
ref:
  <<: *default
```

base.yml

```yaml
_directives:
  include:
    - root.yml
hoge: override
base: ${LLANG:10} # base comment
obj:
  key: 222
arr:
  - 0hoge
  - 1hoge
  - 2hoge
  - 3hoge
```

root.yml

```yaml
root: root
```

generates

```bash
$ yammy generate -i test.yml -c
anchors: #  test.yml:11
    default: &default
        user: USER_ENV_VAR_VALUE #  test.yml:13
arr: #  base.yml:8
    - 0hoge #  base.yml:9
    - 1hoge #  base.yml:10
    - 2hoge #  base.yml:11
    - 3hoge #  base.yml:12
    - 01hoge #  test.yml:10
base: 10 # base comment base.yml:5
# bbb
fooo: 111 #ddd test.yml:17
hoge: value #  test.yml:15
obj: #  base.yml:6
    key: 333 #  test.yml:7
# ccc
ref: #  test.yml:19
    !!merge <<: *default #  test.yml:20
root: root #  root.yml:1
```

With `-k`, yammy generates a file keeping variable expressions. These variable default values are updated with variable values at the time of generation.

```bash

### Go library

You can load YAML/JSON files with `Load` funtion.

```go
func Load(name string, dest any, opts ...LoadOption) error
```

With `WithSourceMapKey` option, you can map a source map node to your struct.

Example: validation error with original source position

```go
type Config struct {
	SourceMap *yammy.SourceMap `yaml:"sourcemap"`
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

func load() {
	var c Config
	err := yammy.Load("config.yml",
		&c,
		WithSourceMapKey("sourcemap"))
}
```

yammy uses `gopkg.in/yaml.v3` as a YAML/JSON library, so you can use struct tags that is
defined in `gopkg.in/yaml.v3`.


## Donation
BTC: 1NEDSyUmo4SMTDP83JJQSWi1MvQUGGNMZB

## License
MIT

## Author
Yusuke Inuzuka
