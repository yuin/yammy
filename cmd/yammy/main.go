// Package main is an entry point for the yammy command.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/yuin/yammy"
	"gopkg.in/yaml.v3"
)

func abortIf(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func main() {
	generateCmd := flag.NewFlagSet("generate", flag.ExitOnError)
	generateHelp := generateCmd.Bool("h", false, "show this help")
	generateInput := generateCmd.String("i", "", "source file path(required)")
	generateOutput := generateCmd.String("o", "", "output file path(optional)")
	generateFormat := generateCmd.String("f", "yaml", "output format(yaml or json)")
	generateSourceMapComment := generateCmd.Bool("c", false, "add source map comments")
	generateKeepsVariables := generateCmd.Bool("k", false, "keep variable expressions(optional)")
	generateRemovesBlockComments := generateCmd.Bool("b", false, "remove block comments(optional)")
	generateSourceMap := generateCmd.String("s", "", "source map node key name")
	generateEnvJSONPatches := generateCmd.String("p", "JSON_PATCH", "JSON Patch env key prefix")

	cmdName := "generate"
	args := []string{}
	if len(os.Args) > 1 {
		cmdName = os.Args[1]
		args = os.Args[2:]
	}
redo:

	switch cmdName {
	case "generate":
		abortIf(generateCmd.Parse(args))
		if *generateHelp {
			generateCmd.Usage()
			os.Exit(1)
		}
		if len(*generateInput) == 0 {
			generateCmd.Usage()
			os.Exit(1)
		}
		if *generateFormat != "yaml" && *generateFormat != "json" {
			generateCmd.Usage()
			os.Exit(1)
		}

		var n yaml.Node
		var opts []yammy.LoadOption
		if *generateSourceMapComment {
			opts = append(opts, yammy.WithSourceMapComment())
		}
		if len(*generateSourceMap) != 0 {
			opts = append(opts, yammy.WithSourceMapKey(*generateSourceMap))
		}
		if *generateKeepsVariables {
			opts = append(opts, yammy.WithKeepsVariables())
		}
		if *generateRemovesBlockComments {
			opts = append(opts, yammy.WithRemovesBlockComments())
		}
		if len(*generateEnvJSONPatches) != 0 {
			opts = append(opts, yammy.WithEnvJSONPatches(*generateEnvJSONPatches))
		}
		abortIf(yammy.Load(*generateInput, &n, opts...))
		var err error
		var bs []byte
		switch *generateFormat {
		case "yaml":
			bs, err = yaml.Marshal(&n)
		case "json":
			var m map[string]any
			abortIf(n.Decode(&m))
			bs, err = json.MarshalIndent(m, "", "  ")
		}
		abortIf(err)
		if len(*generateOutput) != 0 {
			abortIf(os.WriteFile(*generateOutput, bs, 0660))
		} else {
			fmt.Println(string(bs))
		}
		os.Exit(0)
	case "-h":
		fmt.Fprint(os.Stderr, `yammy [COMMAND|-h]
  COMMANDS: (default: generate)
    generate: generates a YAML/JSON file
  OPTIONS:
    -h: show this help
`)
		os.Exit(1)
	default:
		cmdName = "generate"
		args = os.Args[1:]
		goto redo
	}
}
