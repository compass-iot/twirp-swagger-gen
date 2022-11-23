package main

import (
	"errors"
	"flag"
	"fmt"
	"path/filepath"

	"github.com/apex/log"
	"github.com/davecgh/go-spew/spew"
	"github.com/go-bridget/twirp-swagger-gen/internal/swagger"
	"google.golang.org/protobuf/compiler/protogen"
)

var _ = spew.Dump

func init() {
	log.SetLevel(log.InfoLevel)
}

func errorIfEmpty(key string, value *string) error {
	if value == nil {
		return fmt.Errorf("%s is nil", key)
	}
	if *value == "" {
		return fmt.Errorf("%s is empty", key)
	}
	return nil
}

func main() {
	var flags flag.FlagSet
	hostname := flags.String("hostname", "", "")
	pathPrefix := flags.String("path_prefix", "/twirp", "")
	outputSuffix := flags.String("output_suffix", ".swagger.json", "")

	// Extra args for Compass IoT
	version := flags.String("version", "", "")
	sdkfiles := flags.String("sdk_files", "", "")
	protoDir := flags.String("proto_dir", "", "")
	templateDir := flags.String("template_dir", "", "")
	outDir := flags.String("out_dir", "", "")

	opts := &protogen.Options{
		ParamFunc: flags.Set,
	}

	opts.Run(func(gen *protogen.Plugin) error {
		for _, f := range gen.Files {
			in := f.Desc.Path()
			log.Debugf("generating: %q", in)

			if !f.Generate {
				log.Debugf("skip generating: %q", in)
				continue
			}

			// Check required args
			if err := errorIfEmpty("hostname", hostname); err != nil {
				return err
			}
			if err := errorIfEmpty("version", version); err != nil {
				return err
			}
			if err := errorIfEmpty("sdk_files", sdkfiles); err != nil {
				return err
			}
			if err := errorIfEmpty("proto_dir", protoDir); err != nil {
				return err
			}
			if err := errorIfEmpty("template_dir", templateDir); err != nil {
				return err
			}

			writer := swagger.NewWriter(in, *hostname, *pathPrefix, *version, *sdkfiles, *protoDir, *templateDir)
			if err := writer.WalkFile(); err != nil {
				if errors.Is(err, swagger.ErrNoServiceDefinition) {
					log.Debugf("skip writing file, %s: %q", err, in)
					continue
				}
				return err
			}

			out := *outDir + filepath.Base(f.GeneratedFilenamePrefix) + *outputSuffix
			g := gen.NewGeneratedFile(out, f.GoImportPath)
			if _, err := g.Write(writer.Get()); err != nil {
				return err
			}
		}
		return nil
	})
}
