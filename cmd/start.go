// Copyright © 2016 Valere JEANTET <valere.jeantet@gmail.com>
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

package cmd

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/veino/config"
	"github.com/veino/runtime"
	"github.com/veino/runtime/metrics"
)

func startLogfan(flagConfigPath string, flagConfigContent string, stats metrics.IStats) error {
	runtime.SetIStat(stats)
	runtime.Start(webhookListen)
	runtime.Logger().SetVerboseMode(verbose)
	runtime.Logger().SetDebugMode(debug)
	var configAgents = []config.Agent{}

	// Load agents from flagConfigContent string
	if flagConfigContent != "" {
		fileConfigAgents, err := parseConfig("inline", []byte(flagConfigContent))
		if err != nil {
			return fmt.Errorf("ERROR while using config. %s", err.Error())
		}
		configAgents = append(configAgents, fileConfigAgents...)
	}

	// Load all agents configuration from conf files
	if flagConfigPath != "" {
		if fi, err := os.Stat(flagConfigPath); err == nil {
			if fi.IsDir() {
				flagConfigPath = flagConfigPath + string(os.PathSeparator) + "*.conf"
			}
		} else {
			return fmt.Errorf("ERROR %s", err.Error())
		}

		//List all conf files if flagConfigPath folder
		files, err := filepath.Glob(flagConfigPath)
		if err != nil {
			return fmt.Errorf("error %s", err.Error())
		}

		//use each file
		for _, file := range files {
			var fileConfigAgents []config.Agent
			content, err := ioutil.ReadFile(file)
			if err != nil {
				log.Printf(`Error while reading "%s" [%s]`, file, err)
				continue
			}
			// instance all AgenConfiguration structs from file content
			var filename = filepath.Base(file)
			var extension = filepath.Ext(filename)
			var pipelineName = filename[0 : len(filename)-len(extension)]
			fileConfigAgents, err = parseConfig(pipelineName, content)
			if err != nil {
				break
			}
			log.Printf("using config file : %s\n", file)
			if err != nil {
				return fmt.Errorf("error %s", err.Error())
			}
			configAgents = append(configAgents, fileConfigAgents...)
		}
	}
	runtime.StartAgents(configAgents)
	return nil
}

func startLogfanAndWait(flagConfigPath string, flagConfigContent string, stats metrics.IStats) {
	ch := make(chan os.Signal)
	err := startLogfan(flagConfigPath, flagConfigContent, stats)
	if err != nil {
		log.Fatal(err)
	}
	log.Print("Logfan started")
	// Wait for signal CTRL+C for send a stop event to all AgentProcessor
	// When CTRL+C, SIGINT and SIGTERM signal occurs
	// Then stop server gracefully
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	log.Println(<-ch)
	log.Printf("stopping...")
	runtime.Stop()
	log.Printf("Everything stopped gracefully. Goodbye!\n")
}
