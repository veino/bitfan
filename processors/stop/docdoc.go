// Code generated by "bitfanDoc "; DO NOT EDIT
package stopprocessor

import "github.com/vjeantet/bitfan/processors/doc"

func (p *processor) Doc() *doc.Processor {
	return &doc.Processor{
  Name:       "stopprocessor",
  ImportPath: "/Users/sodadi/go/src/github.com/vjeantet/bitfan/processors/stop",
  Doc:        "Stop after emitting a blank event on start\nAllow you to put first event and then stop processors as soon as they finish their job.\n\nPermit to launch bitfan with a pipeline and quit when work is done.",
  DocShort:   "",
  Options:    &doc.ProcessorOptions{
    Doc:     "",
    Options: []*doc.ProcessorOption{
      &doc.ProcessorOption{
        Name:           "Add_field",
        Alias:          "",
        Doc:            "If this filter is successful, add any arbitrary fields to this event.",
        Required:       false,
        Type:           "hash",
        DefaultValue:   nil,
        PossibleValues: []string{},
        ExampleLS:      "",
      },
      &doc.ProcessorOption{
        Name:           "Tags",
        Alias:          "",
        Doc:            "If this filter is successful, add arbitrary tags to the event. Tags can be dynamic\nand include parts of the event using the %{field} syntax.",
        Required:       false,
        Type:           "array",
        DefaultValue:   nil,
        PossibleValues: []string{},
        ExampleLS:      "",
      },
      &doc.ProcessorOption{
        Name:           "Type",
        Alias:          "",
        Doc:            "Add a type field to all events handled by this input",
        Required:       false,
        Type:           "string",
        DefaultValue:   nil,
        PossibleValues: []string{},
        ExampleLS:      "",
      },
      &doc.ProcessorOption{
        Name:           "ExitBitfan",
        Alias:          "exit_bitfan",
        Doc:            "Stop bitfan after stopping the pipeline ?",
        Required:       false,
        Type:           "bool",
        DefaultValue:   "true",
        PossibleValues: []string{},
        ExampleLS:      "",
      },
    },
  },
  Ports: []*doc.ProcessorPort{},
}
}