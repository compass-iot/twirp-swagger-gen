package main

import (
	"flag"

	"github.com/apex/log"
	"github.com/davecgh/go-spew/spew"
	"github.com/go-bridget/twirp-swagger-gen/internal/swagger"
	"github.com/pkg/errors"
)

var _ = spew.Dump

func parse(hostname, filename, output, prefix, version, sdkfiles, protoDir, templateDir string) error {
	if filename == output {
		return errors.New("output file must be different than input file")
	}

	writer := swagger.NewWriter(filename, hostname, prefix, version, sdkfiles, protoDir, templateDir)
	if err := writer.WalkFile(); err != nil {
		if !errors.Is(err, swagger.ErrNoServiceDefinition) {
			return err
		}
	}
	return writer.Save(output)
}

func main() {
	var (
		in          string
		out         string
		host        string
		pathPrefix  string
		version     string
		sdkfiles    string
		protoDir    string
		templateDir string
	)
	flag.StringVar(&in, "in", "", "Input source .proto file")
	flag.StringVar(&out, "out", "", "Output swagger.json file")
	flag.StringVar(&host, "host", "api.example.com", "API host name")
	flag.StringVar(&pathPrefix, "pathPrefix", "/twirp", "Twrirp server path prefix")
	flag.StringVar(&version, "version", "", "API version")
	flag.StringVar(&sdkfiles, "sdk_files", "", "Comma-separated values of linked SDK files")
	flag.StringVar(&protoDir, "proto_dir", "", "Directory of proto files")
	flag.StringVar(&templateDir, "template_dir", "", "Directory of template files")
	flag.Parse()

	if in == "" {
		log.Fatalf("Missing parameter: -in [input.proto]")
	}
	if out == "" {
		log.Fatalf("Missing parameter: -out [output.proto]")
	}
	if host == "" {
		log.Fatalf("Missing parameter: -host [api.example.com]")
	}
	if version == "" {
		log.Fatalf("Missing parameter: -version [0.0.0]")
	}
	if sdkfiles == "" {
		log.Fatalf("Missing parameter: -sdk_files [api1.ts,api2.py]")
	}
	if protoDir == "" {
		log.Fatalf("Missing parameter: -proto_dir [/protos]")
	}
	if templateDir == "" {
		log.Fatalf("Missing parameter: -template_dir [/templates]")
	}

	if err := parse(host, in, out, pathPrefix, version, sdkfiles, protoDir, templateDir); err != nil {
		log.WithError(err).Fatal("exit with error")
	}
}
