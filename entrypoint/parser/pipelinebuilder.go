package parser

import (
	"bytes"
	"fmt"
	"strconv"

	"github.com/vjeantet/bitfan/core/config"
	"github.com/vjeantet/bitfan/entrypoint/parser/logstash"
)

var entryPointContent func(string, string,map[string]interface{}) ([]byte, string, error)

func parseConfigLocation(path string, options map[string]interface{}, pwd string, pickSections ...string) ([]config.Agent, error) {
	if path == "" {
		return []config.Agent{}, fmt.Errorf("no location provided to get content from ; options=%v ", options)
	}

	content, cwd, err := entryPointContent(path, pwd,options)

	if err != nil {
		return nil, err
	}

	agents, err := buildAgents(content, cwd, pickSections...)
	return agents, err
}

func BuildAgents(content []byte, pwd string, contentProvider func(string, string, map[string]interface{}) ([]byte, string, error)) ([]config.Agent, error) {
	entryPointContent = contentProvider
	return buildAgents(content, pwd)
}

func buildAgents(content []byte, pwd string, pickSections ...string) ([]config.Agent, error) {
	var i int
	agentConfList := []config.Agent{}
	if len(pickSections) == 0 {
		pickSections = []string{"input", "filter", "output"}
	}

	p := logstash.NewParser(bytes.NewReader(content))

	LSConfiguration, err := p.Parse()

	if err != nil {
		return agentConfList, err
	}

	outPorts := []config.Port{}

	if _, ok := LSConfiguration.Sections["input"]; ok && isInSlice("input", pickSections) {
		for pluginIndex := 0; pluginIndex < len(LSConfiguration.Sections["input"].Plugins); pluginIndex++ {
			plugin := LSConfiguration.Sections["input"].Plugins[pluginIndex]

			agents, tmpOutPorts, err := buildInputAgents(plugin, pwd)
			if err != nil {
				return nil, err
			}

			agentConfList = append(agents, agentConfList...)
			outPorts = append(outPorts, tmpOutPorts...)
		}
	}

	if _, ok := LSConfiguration.Sections["filter"]; ok && isInSlice("filter", pickSections) {
		if _, ok := LSConfiguration.Sections["filter"]; ok {
			for pluginIndex := 0; pluginIndex < len(LSConfiguration.Sections["filter"].Plugins); pluginIndex++ {
				var agents []config.Agent
				i++
				plugin := LSConfiguration.Sections["filter"].Plugins[pluginIndex]
				agents, outPorts, err = buildFilterAgents(plugin, outPorts, pwd)
				if err != nil {
					return nil, err
				}

				agentConfList = append(agents, agentConfList...)
			}
		}
	}

	if _, ok := LSConfiguration.Sections["output"]; ok && isInSlice("output", pickSections) {
		for pluginIndex := 0; pluginIndex < len(LSConfiguration.Sections["output"].Plugins); pluginIndex++ {
			var agents []config.Agent
			i++
			plugin := LSConfiguration.Sections["output"].Plugins[pluginIndex]
			agents, err = buildOutputAgents(plugin, outPorts, pwd)
			if err != nil {
				return nil, err
			}

			agentConfList = append(agents, agentConfList...)
		}
	}

	return agentConfList, nil
}

// TODO : this should return ports to be able to use multiple path use
func buildInputAgents(plugin *logstash.Plugin, pwd string) ([]config.Agent, []config.Port, error) {

	var agent config.Agent
	agent = config.NewAgent()
	agent.Type = "input_" + plugin.Name
	if plugin.Label == "" {
		agent.Label = plugin.Name
	} else {
		agent.Label = plugin.Label
	}
	agent.Buffer = 20
	agent.PoolSize = 1
	agent.Wd = pwd

	// Plugin configuration
	agent.Options = map[string]interface{}{}
	for _, setting := range plugin.Settings {
		agent.Options[setting.K] = setting.V
	}

	// handle codec
	if len(plugin.Codecs) > 0 {
		codecs := map[int]interface{}{}
		for i, codec := range plugin.Codecs {
			if codec.Name != "" {
				pcodec := config.NewCodec(codec.Name)
				for _, setting := range codec.Settings {
					pcodec.Options[setting.K] = setting.V
					if setting.K == "role" {
						pcodec.Role = setting.V.(string)
					}
				}

				codecs[i] = pcodec
			}
		}
		agent.Options["codecs"] = codecs
	}

	// If agent is a "use"
	// build imported pipeline from path
	// connect import plugin Xsource to imported pipeline output
	if plugin.Name == "use" {
		if v, ok := agent.Options["path"]; ok {
			switch v.(type) {
			case string:
				agent.Options["path"] = []string{v.(string)}
				fileConfigAgents, err := parseConfigLocation(v.(string), agent.Options, pwd, "input", "filter")
				if err != nil {
					return nil, nil, err
				}

				// add agent "use" - set use agent Source as last From FileConfigAgents
				inPort := config.Port{AgentID: fileConfigAgents[0].ID, PortNumber: 0}
				agent.AgentSources = append(agent.AgentSources, inPort)
				fileConfigAgents = append([]config.Agent{agent}, fileConfigAgents...)

				outPort := config.Port{AgentID: fileConfigAgents[0].ID, PortNumber: 0}
				return fileConfigAgents, []config.Port{outPort}, nil
			case []interface{}:
				CombinedFileConfigAgents := []config.Agent{}
				newOutPorts := []config.Port{}
				for _, p := range v.([]interface{}) {
					// contruire le pipeline a
					fileConfigAgents, err := parseConfigLocation(p.(string), agent.Options, pwd, "input", "filter")
					if err != nil {
						return nil, nil, err
					}

					// save pipeline a for later return
					CombinedFileConfigAgents = append(CombinedFileConfigAgents, fileConfigAgents...)

					// add agent "use" - set use agent Source as last From FileConfigAgents
					inPort := config.Port{AgentID: fileConfigAgents[0].ID, PortNumber: 0}
					newOutPorts = append(newOutPorts, inPort)
				}

				// connect all collected inPort to "use" agent
				agent.AgentSources = append(agent.AgentSources, newOutPorts...)

				// add "use" plugin to combined pipelines
				CombinedFileConfigAgents = append([]config.Agent{agent}, CombinedFileConfigAgents...)

				// return  pipeline a b c ... with theirs respectives outputs
				return CombinedFileConfigAgents, []config.Port{{AgentID: agent.ID, PortNumber: 0}}, nil
			}
		}
	}

	// interval can be a number, a string number or a cron string pattern
	interval := agent.Options["interval"]
	switch t := interval.(type) {
	case int, int8, int16, int32, int64:
		agent.Schedule = fmt.Sprintf("@every %ds", t)
	case string:
		if i, err := strconv.Atoi(t); err == nil {
			agent.Schedule = fmt.Sprintf("@every %ds", i)
		} else {
			agent.Schedule = t
		}
	}

	// @see commit dbeb4015a88893bffd6334d38f34f978312eff82
	if trace, ok := agent.Options["trace"]; ok {
		switch t := trace.(type) {
		case string:
			agent.Trace = true
		case bool:
			agent.Trace = t
		}
	}

	if workers, ok := agent.Options["workers"]; ok {
		switch t := workers.(type) {
		case int64:
			agent.PoolSize = int(t)
		case int32:
			agent.PoolSize = int(t)
		case string:
			if i, err := strconv.Atoi(t); err == nil {
				agent.PoolSize = i
			}
		}
	}

	outPort := config.Port{AgentID: agent.ID, PortNumber: 0}
	return []config.Agent{agent}, []config.Port{outPort}, nil
}

func buildOutputAgents(plugin *logstash.Plugin, lastOutPorts []config.Port, pwd string) ([]config.Agent, error) {
	agent_list := []config.Agent{}

	var agent config.Agent
	agent = config.NewAgent()
	agent.Type = "output_" + plugin.Name
	if plugin.Label == "" {
		agent.Label = plugin.Name
	} else {
		agent.Label = plugin.Label
	}
	agent.Buffer = 20
	agent.PoolSize = 1
	agent.Wd = pwd

	// Plugin configuration
	agent.Options = map[string]interface{}{}
	for _, setting := range plugin.Settings {
		agent.Options[setting.K] = setting.V
	}

	// handle codec
	if len(plugin.Codecs) > 0 {
		codecs := map[int]interface{}{}
		for i, codec := range plugin.Codecs {
			if codec.Name != "" {
				pcodec := config.NewCodec(codec.Name)
				for _, setting := range codec.Settings {
					pcodec.Options[setting.K] = setting.V
					if setting.K == "role" {
						pcodec.Role = setting.V.(string)
					}
				}
				codecs[i] = pcodec
			}
		}
		agent.Options["codecs"] = codecs
	}

	// if its a use plugin
	// load filter and output parts of pipeline
	// connect pipeline Xsource to lastOutPorts
	// return pipelineagents with lastOutPorts intact
	// handle use plugin
	// If its a use agent
	// build the filter part of the pipeline
	// connect pipeline first agent Xsource to lastOutPorts output
	// return imported pipeline with its output
	if plugin.Name == "use" {
		if v, ok := agent.Options["path"]; ok {
			switch v.(type) {
			case string:
				agent.Options["path"] = []string{v.(string)}
				fileConfigAgents, err := parseConfigLocation(v.(string), agent.Options, pwd, "filter", "output")
				if err != nil {
					return nil, err
				}

				firstUsedAgent := &fileConfigAgents[len(fileConfigAgents)-1]
				for _, sourceport := range lastOutPorts {
					inPort := config.Port{AgentID: sourceport.AgentID, PortNumber: sourceport.PortNumber}
					firstUsedAgent.AgentSources = append(firstUsedAgent.AgentSources, inPort)
				}

				//specific to output
				return fileConfigAgents, nil

			case []interface{}:
				CombinedFileConfigAgents := []config.Agent{}
				for _, p := range v.([]interface{}) {
					fileConfigAgents, err := parseConfigLocation(p.(string), agent.Options, pwd, "filter", "output")
					if err != nil {
						return nil, err
					}

					firstUsedAgent := &fileConfigAgents[len(fileConfigAgents)-1]
					for _, sourceport := range lastOutPorts {
						inPort := config.Port{AgentID: sourceport.AgentID, PortNumber: sourceport.PortNumber}
						firstUsedAgent.AgentSources = append(firstUsedAgent.AgentSources, inPort)
					}
					CombinedFileConfigAgents = append(CombinedFileConfigAgents, fileConfigAgents...)
				}
				// return  pipeline a b c ... with theirs respectives outputs
				return CombinedFileConfigAgents, nil
			}
		}
	}

	// Plugin Sources
	agent.AgentSources = config.PortList{}
	for _, sourceport := range lastOutPorts {
		inPort := config.Port{AgentID: sourceport.AgentID, PortNumber: sourceport.PortNumber}
		agent.AgentSources = append(agent.AgentSources, inPort)
	}

	// if plugin.Codec != nil {
	// 	agent.Options["codec"] = plugin.Codec.Name
	// }

	// Is this Plugin has conditional expressions ?
	if len(plugin.When) > 0 {
		// outPorts_when := []port{}
		// le plugin WHEn est $plugin
		agent.Options["expressions"] = map[int]string{}
		// Loop over expressions in correct order
		for expressionIndex := 0; expressionIndex < len(plugin.When); expressionIndex++ {
			when := plugin.When[expressionIndex]
			//	enregistrer l'expression dans la conf agent
			agent.Options["expressions"].(map[int]string)[expressionIndex] = when.Expression

			// recupérer le outport associé (expressionIndex)
			expressionOutPorts := []config.Port{
				{AgentID: agent.ID, PortNumber: expressionIndex},
			}

			// construire les plugins associés à l'expression
			// en utilisant le expressionOutPorts
			for pi := 0; pi < len(when.Plugins); pi++ {
				p := when.Plugins[pi]
				var agents []config.Agent
				var err error
				// récupérer le dernier outport du plugin créé il devient expressionOutPorts
				agents, err = buildOutputAgents(p, expressionOutPorts, pwd)
				if err != nil {
					return nil, err
				}

				// ajoute l'agent à la liste des agents
				agent_list = append(agents, agent_list...)
			}
		}
	}

	// @see commit dbeb4015a88893bffd6334d38f34f978312eff82
	if trace, ok := agent.Options["trace"]; ok {
		switch t := trace.(type) {
		case string:
			agent.Trace = true
		case bool:
			agent.Trace = t
		}
	}

	// ajoute l'agent à la liste des agents
	agent_list = append([]config.Agent{agent}, agent_list...)
	return agent_list, nil
}

func buildFilterAgents(plugin *logstash.Plugin, lastOutPorts []config.Port, pwd string) ([]config.Agent, []config.Port, error) {

	agent_list := []config.Agent{}

	var agent config.Agent
	agent = config.NewAgent()
	agent.Type = plugin.Name
	if plugin.Label == "" {
		agent.Label = plugin.Name
	} else {
		agent.Label = plugin.Label
	}

	agent.Buffer = 20
	agent.PoolSize = 2
	agent.Wd = pwd

	// Plugin configuration
	agent.Options = map[string]interface{}{}
	for _, setting := range plugin.Settings {
		agent.Options[setting.K] = setting.V
	}

	// handle codec
	if len(plugin.Codecs) > 0 {
		codecs := map[int]interface{}{}
		for i, codec := range plugin.Codecs {
			if codec.Name != "" {
				pcodec := config.NewCodec(codec.Name)
				for _, setting := range codec.Settings {
					pcodec.Options[setting.K] = setting.V
					if setting.K == "role" {
						pcodec.Role = setting.V.(string)
					}
				}
				codecs[i] = pcodec
			}
		}
		agent.Options["codecs"] = codecs
	}

	// handle use plugin
	// If its a use agent
	// build the filter part of the pipeline
	// connect pipeline first agent Xsource to lastOutPorts output
	// return imported pipeline with its output
	if plugin.Name == "use" {
		if v, ok := agent.Options["path"]; ok {
			switch v.(type) {
			case string:
				agent.Options["path"] = []string{v.(string)}
				fileConfigAgents, err := parseConfigLocation(v.(string), agent.Options, pwd, "filter")
				if err != nil {
					return nil, nil, err
				}

				firstUsedAgent := &fileConfigAgents[len(fileConfigAgents)-1]
				for _, sourceport := range lastOutPorts {
					inPort := config.Port{AgentID: sourceport.AgentID, PortNumber: sourceport.PortNumber}
					firstUsedAgent.AgentSources = append(firstUsedAgent.AgentSources, inPort)
				}

				newOutPorts := []config.Port{
					{AgentID: fileConfigAgents[0].ID, PortNumber: 0},
				}
				return fileConfigAgents, newOutPorts, nil

			case []interface{}:
				CombinedFileConfigAgents := []config.Agent{}
				newOutPorts := []config.Port{}
				for _, p := range v.([]interface{}) {
					// contruire le pipeline a
					fileConfigAgents, err := parseConfigLocation(p.(string), agent.Options, pwd, "filter")
					if err != nil {
						return nil, nil, err
					}

					// connect pipeline a first agent Xsource to lastOutPorts output
					firstUsedAgent := &fileConfigAgents[len(fileConfigAgents)-1]
					for _, sourceport := range lastOutPorts {
						inPort := config.Port{AgentID: sourceport.AgentID, PortNumber: sourceport.PortNumber}
						firstUsedAgent.AgentSources = append(firstUsedAgent.AgentSources, inPort)
					}
					// save pipeline a for later return
					CombinedFileConfigAgents = append(CombinedFileConfigAgents, fileConfigAgents...)
					// save pipeline a outputs for later return
					newOutPorts = append(newOutPorts, config.Port{AgentID: fileConfigAgents[0].ID, PortNumber: 0})
				}

				// connect all collected newOutPorts to "use" agent
				agent.AgentSources = append(agent.AgentSources, newOutPorts...)
				CombinedFileConfigAgents = append([]config.Agent{agent}, CombinedFileConfigAgents...)

				// return  pipeline a b c ... with theirs respectives outputs
				return CombinedFileConfigAgents, []config.Port{{AgentID: agent.ID, PortNumber: 0}}, nil
			}
		}
	}

	// route = set a pipeline, but do not reconnect it
	if plugin.Name == "route" {
		CombinedFileConfigAgents := []config.Agent{}
		for _, p := range agent.Options["path"].([]interface{}) {
			fileConfigAgents, err := parseConfigLocation(p.(string), agent.Options, pwd, "filter", "output")
			if err != nil {
				return nil, nil, err
			}

			// connect pipeline a last agent Xsource to lastOutPorts output
			lastUsedAgent := &fileConfigAgents[0]
			lastUsedAgent.AgentSources = append(lastUsedAgent.AgentSources, config.Port{AgentID: agent.ID, PortNumber: 0})

			CombinedFileConfigAgents = append(CombinedFileConfigAgents, fileConfigAgents...)
		}

		// connect route to lastOutPorts
		agent.AgentSources = append(agent.AgentSources, lastOutPorts...)
		// add route to routeedpipelines
		CombinedFileConfigAgents = append(CombinedFileConfigAgents, []config.Agent{agent}...)

		// return untouched outputsPorts
		return CombinedFileConfigAgents, []config.Port{{AgentID: agent.ID, PortNumber: 1}}, nil
	}

	// interval can be a number, a string number or a cron string pattern
	interval := agent.Options["interval"]
	switch t := interval.(type) {
	case int, int8, int16, int32, int64:
		agent.Schedule = fmt.Sprintf("@every %ds", t)
	case string:
		if i, err := strconv.Atoi(t); err == nil {
			agent.Schedule = fmt.Sprintf("@every %ds", i)
		} else {
			agent.Schedule = t
		}
	}

	// @see commit dbeb4015a88893bffd6334d38f34f978312eff82
	if trace, ok := agent.Options["trace"]; ok {
		switch t := trace.(type) {
		case string:
			agent.Trace = true
		case bool:
			agent.Trace = t
		}
	}

	if workers, ok := agent.Options["workers"]; ok {
		switch t := workers.(type) {
		case int64:
			agent.PoolSize = int(t)
		case int32:
			agent.PoolSize = int(t)
		case string:
			if i, err := strconv.Atoi(t); err == nil {
				agent.PoolSize = i
			}
		}
	}

	// Plugin Sources
	agent.AgentSources = config.PortList{}
	for _, sourceport := range lastOutPorts {
		inPort := config.Port{AgentID: sourceport.AgentID, PortNumber: sourceport.PortNumber}
		agent.AgentSources = append(agent.AgentSources, inPort)
	}

	// By Default Agents output to port 0
	newOutPorts := []config.Port{
		{AgentID: agent.ID, PortNumber: 0},
	}

	// Is this Plugin has conditional expressions ?
	if len(plugin.When) > 0 {
		outPorts_when := []config.Port{}
		// le plugin WHEn est $plugin
		agent.Options["expressions"] = map[int]string{}
		elseOK := false
		// Loop over expressions in correct order
		for expressionIndex := 0; expressionIndex < len(plugin.When); expressionIndex++ {
			when := plugin.When[expressionIndex]
			//	enregistrer l'expression dans la conf agent
			agent.Options["expressions"].(map[int]string)[expressionIndex] = when.Expression
			if when.Expression == "true" {
				elseOK = true
			}
			// recupérer le outport associé (expressionIndex)
			expressionOutPorts := []config.Port{
				{AgentID: agent.ID, PortNumber: expressionIndex},
			}

			// construire les plugins associés à l'expression
			// en utilisant le outportA
			for pi := 0; pi < len(when.Plugins); pi++ {
				p := when.Plugins[pi]
				var agents []config.Agent
				var err error
				// récupérer le dernier outport du plugin créé il devient outportA
				agents, expressionOutPorts, err = buildFilterAgents(p, expressionOutPorts, pwd)
				if err != nil {
					return nil, nil, err
				}

				// ajoute l'agent à la liste des agents
				agent_list = append(agents, agent_list...)
			}
			// ajouter le dernier outportA de l'expression au outport final du when
			outPorts_when = append(expressionOutPorts, outPorts_when...)
		}
		newOutPorts = outPorts_when

		// If no else expression was found, insert one
		if elseOK == false {
			agent.Options["expressions"].(map[int]string)[len(agent.Options["expressions"].(map[int]string))] = "true"
			elseOutPorts := []config.Port{
				{AgentID: agent.ID, PortNumber: len(agent.Options["expressions"].(map[int]string)) - 1},
			}
			newOutPorts = append(elseOutPorts, newOutPorts...)
		}
	}

	// ajoute l'agent à la liste des agents
	agent_list = append([]config.Agent{agent}, agent_list...)
	return agent_list, newOutPorts, nil
}

func isInSlice(needle string, candidates []string) bool {
	for _, symbolType := range candidates {
		if needle == symbolType {
			return true
		}
	}
	return false
}
