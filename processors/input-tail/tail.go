//go:generate bitfanDoc
package tail

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ShowMax/go-fqdn"
	zglob "github.com/mattn/go-zglob"

	"github.com/vjeantet/bitfan/codecs"
	"github.com/vjeantet/bitfan/processors"

	"github.com/hpcloud/tail"
	"github.com/hpcloud/tail/watch"
)

func New() processors.Processor {
	return &processor{opt: &options{}}
}

type processor struct {
	processors.Base

	opt                 *options
	sinceDBInfos        map[string]*sinceDBInfo
	sinceDBLastInfosRaw []byte
	sinceDBLastSaveTime time.Time
	q                   chan bool
	wg                  sync.WaitGroup
	sinceDBInfosMutex   *sync.Mutex
	host                string
	watchFiles          map[string]bool
}

type options struct {
	processors.CommonOptions `mapstructure:",squash"`

	// Closes any files that were last read the specified timespan in seconds ago.
	// Default value is 3600 (i.e. 1 hour)
	// This has different implications depending on if a file is being tailed or read.
	// If tailing, and there is a large time gap in incoming data the file can be
	// closed (allowing other files to be opened) but will be queued for reopening
	// when new data is detected. If reading, the file will be closed after
	// close_older seconds from when the last bytes were read.
	// @Default 3600
	CloseOlder int `mapstructure:"close_older"`

	// The codec used for input data. Input codecs are a convenient method for decoding
	// your data before it enters the input, without needing a separate filter in your bitfan pipeline
	// @Type Codec
	// @Default "line"
	Codec codecs.CodecCollection `mapstructure:"codec"`

	// Set the new line delimiter. Default value is "\n"
	// @Default "\n"
	Delimiter string `mapstructure:"delimiter"`

	// How often (in seconds) we expand the filename patterns in the path option
	// to discover new files to watch. Default value is 15
	// @Default 15
	DiscoverInterval int `mapstructure:"discover_interval"`

	// Exclusions (matched against the filename, not full path).
	// Filename patterns are valid here, too.
	Exclude []string `mapstructure:"exclude"`

	// When the file input discovers a file that was last modified before the
	// specified timespan in seconds, the file is ignored.
	// After it’s discovery, if an ignored file is modified it is no longer ignored
	// and any new data is read.
	// Default value is 86400 (i.e. 24 hours)
	// @Default 86400
	IgnoreOlder int `mapstructure:"ignore_older"`

	// What is the maximum number of file_handles that this input consumes at any one time.
	// Use close_older to close some files if you need to process more files than this number.
	MaxOpenFiles string `mapstructure:"max_open_files"`

	// The path(s) to the file(s) to use as an input.
	// You can use filename patterns here, such as /var/log/*.log.
	// If you use a pattern like /var/log/**/*.log, a recursive search of /var/log
	// will be done for all *.log files.
	// Paths must be absolute and cannot be relative.
	// You may also configure multiple paths.
	Path []string `mapstructure:"path" validate:"required"`

	// Path of the sincedb database file
	// The sincedb database keeps track of the current position of monitored
	// log files that will be written to disk.
	// @Default ".sincedb.json"
	SincedbPath string `mapstructure:"sincedb_path"`

	// How often (in seconds) to write a since database with the current position of monitored log files.
	// Default value is 15
	// @Default 15
	SincedbWriteInterval int `mapstructure:"sincedb_write_interval"`

	// Choose where BitFan starts initially reading files: at the beginning or at the end.
	// The default behavior treats files like live streams and thus starts at the end.
	// If you have old data you want to import, set this to beginning.
	// This option only modifies "first contact" situations where a file is new
	// and not seen before, i.e. files that don’t have a current position recorded in a sincedb file.
	// If a file has already been seen before, this option has no effect and the
	// position recorded in the sincedb file will be used.
	// Default value is "end"
	// Value can be any of: "beginning", "end"
	// @Default "end"
	StartPosition string `mapstructure:"start_position"`

	// How often (in seconds) we stat files to see if they have been modified.
	// Increasing this interval will decrease the number of system calls we make,
	// but increase the time to detect new log lines.
	// Default value is 1
	// @Default 1
	StatInterval int `mapstructure:"stat_interval"`
}

func (p *processor) Configure(ctx processors.ProcessorContext, conf map[string]interface{}) error {
	defaults := options{
		StartPosition:        "end",
		SincedbPath:          ".sincedb.json",
		SincedbWriteInterval: 15,
		StatInterval:         1,
		DiscoverInterval:     15,
		Codec: codecs.CodecCollection{
			Dec: codecs.New("line", nil, ctx.Log(), ctx.ConfigWorkingLocation()),
		},
	}
	p.opt = &defaults
	p.host = fqdn.Get()

	err := p.ConfigureAndValidate(ctx, conf, p.opt)

	if false == filepath.IsAbs(p.opt.SincedbPath) {
		p.opt.SincedbPath = filepath.Join(p.DataLocation, p.opt.SincedbPath)
	}

	return err
}

func (p *processor) filesToRead() ([]string, error) {
	p.Logger.Debugf("Start discover files in : %v", p.opt.Path)
	// Fix relative paths
	fixedPaths := []string{}
	for _, path := range p.opt.Path {
		if !filepath.IsAbs(path) {
			path = filepath.Join(p.ConfigWorkingLocation, path)
		}
		fixedPaths = append(fixedPaths, path)
	}

	p.Logger.Debugf("fixedPaths = %v", fixedPaths)

	var matches []string
	// find files
	for _, currentPath := range fixedPaths {
		if currentMatches, err := zglob.Glob(currentPath); err == nil {
			// if currentMatches, err := filepath.Glob(currentPath); err == nil {
			matches = append(matches, currentMatches...)
			continue
		}
		return matches, fmt.Errorf("glob(%q) failed", currentPath)
	}

	// ignore excluded
	if len(p.opt.Exclude) > 0 {
		var matches_tmp []string
		for _, pattern := range p.opt.Exclude {
			for _, name := range matches {
				if match, _ := filepath.Match(pattern, name); match == false {
					matches_tmp = append(matches_tmp, name)
				} else {
					p.Logger.Debugf("scan ignore (exlude) %s", name)
				}
			}
		}
		matches = matches_tmp
	}

	// ignore already seen files
	// var matches_tmp []string
	// for _, name := range matches {
	// 	if !p.sinceDBInfos.has(name) {
	// 		matches_tmp = append(matches_tmp, name)
	// 	} else {
	// 		p.Logger.Debugf("scan ignore (sincedb) %s", name)
	// 	}
	// }
	// matches = matches_tmp

	var matches_tmp []string
	for _, name := range matches {
		info, err := os.Stat(name)
		if err != nil {
			p.Logger.Warnf("Error while stating " + name)
			break
		}
		duration := time.Since(info.ModTime()).Seconds()
		// ignore modified to soon
		if duration > float64(p.opt.IgnoreOlder) {
			// ignore  too old file
			if p.opt.IgnoreOlder > 0 && duration < float64(p.opt.IgnoreOlder) {
				p.Logger.Debugf("scan ignore (too old) %s", name)
			} else {
				matches_tmp = append(matches_tmp, name)
			}
		}
	}
	matches = matches_tmp
	return matches, nil
}

func (p *processor) discoverFilesToRead() error {
	files, err := p.filesToRead()
	if err != nil {
		p.Logger.Error(err)
		return err
	}
	for _, name := range files {
		if _, ok := p.watchFiles[name]; !ok {
			p.watchFiles[name] = true
			p.wg.Add(1)
			go p.tailFile(name, p.q)
			p.Logger.Debugf("Watch on file : %s", name)
		}
	}
	return nil
}

func (p *processor) Start(e processors.IPacket) error {

	watch.POLL_DURATION = time.Second * time.Duration(p.opt.StatInterval)
	p.q = make(chan bool)
	p.watchFiles = make(map[string]bool)

	p.loadSinceDBInfos()
	go func() {
		ticker := time.NewTicker(time.Duration(p.opt.DiscoverInterval) * time.Second)
		for {
			if err := p.discoverFilesToRead(); err != nil {
				p.Logger.Error(err)
			}
			<-ticker.C
		}
	}()

	go p.checkSaveSinceDBInfosLoop()

	return nil
}

func (p *processor) Stop(e processors.IPacket) error {
	close(p.q)
	p.wg.Wait()
	p.saveSinceDBInfos()
	return nil
}

// func (p *processor) Tick(e processors.IPacket) error    { return nil }
// func (p *processor) Receive(e processors.IPacket) error { return nil }

func (p *processor) tailFile(path string, q chan bool) error {
	defer p.wg.Done()
	var (
		since  *sinceDBInfo
		ok     bool
		whence int
	)

	p.sinceDBInfosMutex.Lock()
	if since, ok = p.sinceDBInfos[path]; !ok {
		p.sinceDBInfos[path] = &sinceDBInfo{}
		since = p.sinceDBInfos[path]
	}
	p.sinceDBInfosMutex.Unlock()

	// Default start reading at end
	whence = os.SEEK_END
	// if this is not the first contact with this file set cursor
	if since.Offset != 0 {
		whence = os.SEEK_SET
	} else if p.opt.StartPosition == "beginning" {
		// if this is the first contact and use want to start at the beginning set cursor at 0
		whence = os.SEEK_SET
	}

	t, err := tail.TailFile(path, tail.Config{
		Logger: p.Logger,
		Location: &tail.SeekInfo{
			Offset: since.Offset,
			Whence: whence,
		},
		Follow: true,
		ReOpen: true,
		Poll:   true,
	})
	if err != nil {
		return err
	}

	go func() {
		<-q
		t.Stop()
	}()

	var dec codecs.Decoder
	pr, pw := io.Pipe()

	if dec, err = p.opt.Codec.NewDecoder(pr); err != nil {
		p.Logger.Errorln("decoder error : ", err.Error())
		return err
	}

	go func() {
		for {
			var record interface{}
			if err := dec.Decode(&record); err != nil {
				p.Logger.Errorln("codec error : ", err.Error())
				return
			}
			var e processors.IPacket
			switch v := record.(type) {
			case string:
				e = p.NewPacket(v, map[string]interface{}{
					"host": p.host,
				})
			case map[string]interface{}:
				e = p.NewPacket("", v)
				e.Fields().SetValueForPath(p.host, "host")
			case []interface{}:
				e = p.NewPacket("", map[string]interface{}{
					"host": p.host,
					"data": v,
				})
			default:
				p.Logger.Errorf("Unknow structure %#v", v)
			}

			p.opt.ProcessCommonOptions(e.Fields())
			p.Send(e)
			since.Offset, _ = t.Tell()
			p.checkSaveSinceDBInfos()
		}
	}()

	for line := range t.Lines {
		fmt.Fprintf(pw, "%s\n", line.Text)
	}
	pr.Close()
	pw.Close()

	return nil
}
