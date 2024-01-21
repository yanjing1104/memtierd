// Copyright 2021 Intel Corporation. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/intel/memtierd/pkg/memtier"
	_ "github.com/intel/memtierd/pkg/version"
)

type config struct {
	Policy   memtier.PolicyConfig
	Routines []memtier.RoutineConfig
}

func exit(format string, a ...interface{}) {
	_, _ = fmt.Fprintf(os.Stderr, "memtierd: "+format+"\n", a...)
	os.Exit(1)
}

func loadConfigFile(filename string) (memtier.Policy, []memtier.Routine) {
	configBytes, err := os.ReadFile(filename)
	if err != nil {
		exit("%s", err)
	}

	var config config
	err = memtier.UnmarshalYamlConfig(configBytes, &config)
	if err != nil {
		exit("error in %q: %s", filename, err)
	}

	if config.Policy == (memtier.PolicyConfig{}) {
		exit("error in policy field or missing")
	}

	if config.Policy.Name == "" {
		exit("error in policy.name filed or missing")
	}

	policy, err := memtier.NewPolicy(config.Policy.Name)
	if err != nil {
		exit("%s", err)
	}

	err = policy.SetConfigJSON(config.Policy.Config)
	if err != nil {
		exit("%s", err)
	}

	routines := []memtier.Routine{}
	for _, routineCfg := range config.Routines {
		routine, err := memtier.NewRoutine(routineCfg.Name)
		if err != nil {
			exit("%s", err)
		}
		err = routine.SetConfigJSON(routineCfg.Config)
		if err != nil {
			exit("routine %s: %s", routineCfg.Name, err)
		}
		routines = append(routines, routine)
	}
	return policy, routines
}

func main() {
	memtier.SetLogger(log.New(os.Stderr, "", 0))
	optPrompt := flag.Bool("prompt", false, "launch interactive prompt (ignore other parameters)")
	optConfig := flag.String("config", "", "launch non-interactive mode with config file")
	optConfigDumpJSON := flag.Bool("config-dump-json", false, "dump effective configuration in JSON")
	optDebug := flag.Bool("debug", false, "print debug output")
	optCommandString := flag.String("c", "-", "run command string, \"-\": from stdin (the default), \"\": non-interactive")

	flag.Parse()
	memtier.SetLogDebug(*optDebug)

	if *optPrompt {
		prompt := memtier.NewPrompt("memtierd> ", bufio.NewReader(os.Stdin), bufio.NewWriter(os.Stdout))
		prompt.Interact()
		return
	}

	var policy memtier.Policy
	var routines []memtier.Routine
	if *optConfig != "" {
		policy, routines = loadConfigFile(*optConfig)
	} else {
		exit("missing -prompt or -config")
	}

	if *optConfigDumpJSON {
		fmt.Printf("%s\n", policy.GetConfigJSON())
		os.Exit(0)
	}

	if policy != nil {
		if err := policy.Start(); err != nil {
			exit("error in starting policy: %s", err)
		}
	}

	for r, routine := range routines {
		if policy != nil {
			if err := routine.SetPolicy(policy); err != nil {
				exit("error in setting policy for routine: %s", err)
			}
		}
		if err := routine.Start(); err != nil {
			exit("error in starting routine %d: %s", r+1, err)
		}
	}

	if *optCommandString != "" {
		var prompt *memtier.Prompt
		if *optCommandString == "-" {
			prompt = memtier.NewPrompt("memtierd> ", bufio.NewReader(os.Stdin), bufio.NewWriter(os.Stdout))
			if stdinFileInfo, _ := os.Stdin.Stat(); (stdinFileInfo.Mode() & os.ModeCharDevice) == 0 {
				// Input comes from a pipe.
				// Echo commands after prompt in the interaction to explain outputs.
				prompt.SetEcho(true)
			}
		} else {
			prompt = memtier.NewPrompt("", bufio.NewReader(strings.NewReader(*optCommandString)), bufio.NewWriter(os.Stdout))
		}
		prompt.SetPolicy(policy)
		if len(routines) > 0 {
			prompt.SetRoutines(routines)
		}
		prompt.Interact()
	} else { // *optCommandString == ""
		select {}
	}
}
