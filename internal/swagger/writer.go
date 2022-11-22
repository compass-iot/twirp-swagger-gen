package swagger

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/apex/log"
	"github.com/emicklei/proto"
	"github.com/go-openapi/spec"
)

var ErrNoServiceDefinition = errors.New("no service definition found")

type Writer struct {
	*spec.Swagger

	filename    string
	hostname    string
	pathPrefix  string
	packageName string

	version     string
	sdkfiles    []string
	protoDir    string // "hack" to get around import resolution issues in proto
	templateDir string
}

func NewWriter(filename, hostname, pathPrefix, version, sdkfiles, protoDir, templateDir string) *Writer {
	if pathPrefix == "" {
		pathPrefix = "/twirp"
	}
	return &Writer{
		filename:    filename,
		hostname:    hostname,
		pathPrefix:  pathPrefix,
		version:     version,
		sdkfiles:    strings.Split(sdkfiles, ","),
		protoDir:    protoDir,
		templateDir: templateDir,
		Swagger:     &spec.Swagger{},
	}
}

func (sw *Writer) Package(pkg *proto.Package) {
	sw.Swagger.Swagger = "2.0"
	sw.Schemes = []string{"https"}
	sw.Produces = []string{"application/json"}
	sw.Host = sw.hostname
	sw.Consumes = sw.Produces

	oauth := make(map[string][]string)
	oauth["oauth"] = []string{}
	sw.Security = make([]map[string][]string, 0)
	sw.Security = append(sw.Security, oauth)

	secDef := make(spec.SecurityDefinitions)
	secDef["oauth"] = &spec.SecurityScheme{
		SecuritySchemeProps: spec.SecuritySchemeProps{
			Description: "Please use [client credentials](https://datatracker.ietf.org/doc/html/rfc6749#section-4.4) given to you by Compass IOT, please only use [basic auth](https://en.wikipedia.org/wiki/Basic_access_authentication) via the 'Authorization' header to obtain access tokens",
			Type:        "oauth2",
			Flow:        "application",
			TokenURL:    path.Join(sw.hostname, "auth"), // final form should be https://api.compassiot.cloud/auth
			Scopes:      make(map[string]string),
		},
	}
	sw.SecurityDefinitions = secDef

	sw.Info = &spec.Info{
		InfoProps: spec.InfoProps{
			Title:       filepath.Base(sw.filename), // anything to do with files, use filepath
			Version:     sw.version,
			Description: sw.MakeDescription(),
		},
		VendorExtensible: spec.VendorExtensible{
			Extensions: sw.MakeLogo(),
		},
	}
	sw.Swagger.Definitions = make(spec.Definitions)
	sw.Swagger.Paths = &spec.Paths{
		Paths: make(map[string]spec.PathItem),
	}
	sw.Tags = make([]spec.Tag, 0)

	sw.packageName = pkg.Name
}

func (sw *Writer) Import(i *proto.Import) {
	// the exclusion here is more about path traversal than it is
	// about the structure of google proto messages. The annotations
	// could serve to document a REST API, which goes beyond what
	// Twitch RPC does out of the box.
	if strings.Contains(i.Filename, "google/api/annotations.proto") {
		return
	}

	// timestamps are handled as string of date-time
	if strings.Contains(i.Filename, "google/protobuf/timestamp.proto") {
		return
	}

	// wrapper types are defined in aliases.go
	if strings.Contains(i.Filename, "google/protobuf/wrappers.proto") {
		return
	}

	log.Debugf("importing %s", i.Filename)

	definition, err := sw.loadProtoFile(i.Filename)
	if err != nil {
		log.Infof("Can't load %s, err=%s, ignoring (want to make PR?)", i.Filename, err)
		return
	}

	oldPackageName := sw.packageName

	withPackage := func(pkg *proto.Package) {
		sw.packageName = pkg.Name
	}

	// additional files walked for messages and imports only
	proto.Walk(definition, proto.WithPackage(withPackage), proto.WithImport(sw.Import), proto.WithMessage(sw.Message), proto.WithEnum(sw.Enum))

	sw.packageName = oldPackageName
}

func comment(comment *proto.Comment) (string, interface{}) {
	if comment == nil {
		return "", ""
	}

	result := ""
	for _, line := range comment.Lines {
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		result += " " + line
	}
	if len(result) > 1 {
		x := strings.Split(result[1:], ";")
		if len(x) > 1 {
			example := strings.TrimSpace(x[1])
			n, err := strconv.Atoi(example)
			if err == nil {
				return x[0], n
			}
			f, err := strconv.ParseFloat(example, 64)
			if err == nil {
				return x[0], f
			}
			return x[0], example
		}
		return x[0], ""
	}
	return "", ""
}

func description(comment *proto.Comment) string {
	if comment == nil {
		return ""
	}

	result := []string{}
	for _, line := range comment.Lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, ";") {
			x := strings.Split(line, ";")
			line = x[0]
		}
		result = append(result, line)
	}
	return strings.Join(result, "\n")
}

func (sw *Writer) RPC(rpc *proto.RPC) {
	parent, ok := rpc.Parent.(*proto.Service)
	if !ok {
		panic("parent is not proto.service")
	}

	//pathName := filepath.Join("/"+sw.pathPrefix+"/", sw.packageName+"."+parent.Name, rpc.Name)
	base := strings.ReplaceAll(strings.ToLower(parent.Name), "service", "")
	pathName := fmt.Sprintf("/%s/%s.%s/%s", base, sw.packageName, parent.Name, rpc.Name)

	summary := description(rpc.Comment)
	sw.Swagger.Paths.Paths[pathName] = spec.PathItem{
		PathItemProps: spec.PathItemProps{
			Post: &spec.Operation{
				OperationProps: spec.OperationProps{
					ID:      rpc.Name,
					Tags:    []string{parent.Name},
					Summary: summary,
					Responses: &spec.Responses{
						ResponsesProps: spec.ResponsesProps{
							StatusCodeResponses: map[int]spec.Response{
								200: {
									ResponseProps: spec.ResponseProps{
										Description: "A successful response.",
										Schema: &spec.Schema{
											SchemaProps: spec.SchemaProps{
												Ref: spec.MustCreateRef(fmt.Sprintf("#/definitions/%s.%s", sw.packageName, rpc.ReturnsType)),
											},
										},
									},
								},
							},
						},
					},
					Parameters: []spec.Parameter{
						{
							ParamProps: spec.ParamProps{
								Name:     "body",
								In:       "body",
								Required: true,
								Schema: &spec.Schema{
									SchemaProps: spec.SchemaProps{
										Ref: spec.MustCreateRef(fmt.Sprintf("#/definitions/%s.%s", sw.packageName, rpc.RequestType)),
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func (sw *Writer) Message(msg *proto.Message) {
	definitionName := fmt.Sprintf("%s.%s", sw.packageName, msg.Name)

	schemaProps := make(map[string]spec.Schema)

	var allowedValues = []string{
		"boolean",
		"integer",
		"number",
		"object",
		"string",
		"bytes",
	}

	find := func(haystack []string, needle string) (int, bool) {
		for k, v := range haystack {
			if v == needle {
				return k, true
			}
		}
		return -1, false
	}

	var fieldOrder = []string{}

	allFields := msg.Elements

	for _, element := range msg.Elements {
		switch val := element.(type) {
		case *proto.Oneof:
			// We're unpacking val.Elements into the field list,
			// which may or may not be correct. The oneof semantics
			// likely bring in edge-cases.
			allFields = append(allFields, val.Elements...)
		default:
			// No need to unpack for *proto.NormalField,...
			log.Debugf("prepare: uknown field type: %T", element)
		}
	}

	addField := func(field *proto.Field, mapKeyType string, repeated bool, order int) {
		var additionalProps *spec.SchemaOrBool
		fieldTitle, example := comment(field.Comment)
		var (
			fieldDescription = description(field.Comment)
			fieldName        = field.Name
			fieldType        = field.Type
			fieldFormat      = field.Type
		)

		p, ok := typeAliases[fieldType]
		if ok {
			fieldType = p.Type
			fieldFormat = p.Format
		}
		if fieldType == fieldFormat {
			fieldFormat = ""
		}

		if mapKeyType != "" {
			p, ok := typeAliases[mapKeyType]
			if ok {
				// doesn't handle map<string, Message> only map<string, primitive>
				additionalProps = &spec.SchemaOrBool{
					Allows: false,
					Schema: &spec.Schema{
						VendorExtensible: spec.VendorExtensible{},
						SchemaProps: spec.SchemaProps{
							Type: []string{p.Type},
						},
						SwaggerSchemaProps: spec.SwaggerSchemaProps{},
					},
				}
				fieldType = "object"
				fieldFormat = ""
			}
		}

		fieldOrder = append(fieldOrder, fieldName)

		ext := make(spec.Extensions)
		ext.Add("x-order", strconv.Itoa(order))

		if _, ok := find(allowedValues, fieldType); ok {
			fieldSchema := spec.Schema{
				SchemaProps: spec.SchemaProps{
					Title:                fieldTitle,
					Description:          fieldDescription,
					Type:                 spec.StringOrArray([]string{fieldType}),
					Format:               fieldFormat,
					AdditionalProperties: additionalProps,
				},
			}
			if repeated {
				fieldSchema.Title = ""
				fieldSchema.Description = ""
				fieldSchema.Format = ""
				schemaProps[fieldName] = spec.Schema{
					SchemaProps: spec.SchemaProps{
						Title:       fieldTitle,
						Description: fieldDescription,
						Type:        spec.StringOrArray([]string{"array"}),
						Format:      fieldFormat,
						Items: &spec.SchemaOrArray{
							Schema: &fieldSchema,
						},
					},
					VendorExtensible: spec.VendorExtensible{
						Extensions: ext,
					},
				}
				if example != "" {
					fieldSchema.WithExample(example)
				}
			} else {
				schemaProps[fieldName] = fieldSchema
			}
			return
		}

		// Prefix rich type with package name
		if !strings.Contains(fieldType, ".") {
			fieldType = sw.packageName + "." + fieldType
		}
		ref := fmt.Sprintf("#/definitions/%s", fieldType)

		if repeated {
			fieldSchema := spec.Schema{
				SchemaProps: spec.SchemaProps{
					Title:       fieldTitle,
					Description: fieldDescription,
					Type:        spec.StringOrArray([]string{"array"}),
					Items: &spec.SchemaOrArray{
						Schema: &spec.Schema{
							SchemaProps: spec.SchemaProps{
								Ref: spec.MustCreateRef(ref),
							},
						},
					},
				},
				VendorExtensible: spec.VendorExtensible{
					Extensions: ext,
				},
			}
			if example != "" {
				fieldSchema.WithExample(example)
			}
			schemaProps[fieldName] = fieldSchema
			return
		}
		fieldSchema := spec.Schema{
			SchemaProps: spec.SchemaProps{
				Title:       fieldTitle,
				Description: fieldDescription,
				Ref:         spec.MustCreateRef(ref),
			},
			VendorExtensible: spec.VendorExtensible{
				Extensions: ext,
			},
		}
		if example != "" {
			fieldSchema.WithExample(example)
		}
		schemaProps[fieldName] = fieldSchema
	}

	for i, element := range allFields {
		switch val := element.(type) {
		case *proto.Comment:
		case *proto.Oneof:
			// Nothing.
		case *proto.OneOfField:
			addField(val.Field, "", false, i)
		case *proto.MapField:
			addField(val.Field, val.KeyType, false, i)
		case *proto.NormalField:
			addField(val.Field, "", val.Repeated, i)
		default:
			log.Infof("Unknown field type: %T", element)
		}
	}

	schemaDesc := description(msg.Comment)
	if len(fieldOrder) > 0 {
		// This is required to infer order, as json object keys
		// don't keep their order. Should have been an array.
		schemaDesc = schemaDesc + "\n\nFields: " + strings.Join(fieldOrder, ", ")
	}

	title, _ := comment(msg.Comment)
	sw.Swagger.Definitions[definitionName] = spec.Schema{
		SchemaProps: spec.SchemaProps{
			Title:       title,
			Description: strings.TrimSpace(schemaDesc),
			Type:        spec.StringOrArray([]string{"object"}),
			Properties:  schemaProps,
		},
	}
}

func (sw *Writer) Enum(msg *proto.Enum) {
	definitionName := fmt.Sprintf("%s.%s", sw.packageName, msg.Name)

	values := make([]interface{}, 0)

	for _, element := range msg.Elements {
		switch val := element.(type) {
		case *proto.EnumField:
			values = append(values, val.Name)
		default:
			log.Infof("Unknown field type: %T", element)
		}
	}

	title, _ := comment(msg.Comment)
	fieldSchema := spec.Schema{
		SchemaProps: spec.SchemaProps{
			Title:       title,
			Description: description(msg.Comment),
			Type:        spec.StringOrArray([]string{"string"}),
			Enum:        values,
		},
	}
	sw.Swagger.Definitions[definitionName] = fieldSchema
}

func (sw *Writer) Service(srv *proto.Service) {
	exists := false
	for _, tag := range sw.Tags {
		if tag.Name == srv.Name {
			exists = true
		}
	}
	if !exists {
		//summary := ""
		//if srv.Comment != nil {
		//	summary = strings.Join(srv.Comment.Lines, "\n")
		//}
		summary := description(srv.Comment)
		tag := spec.Tag{
			TagProps: spec.TagProps{
				Name:        srv.Name,
				Description: summary,
			},
		}
		sw.Tags = append(sw.Tags, tag)
	}
}

func (sw *Writer) Handlers() []proto.Handler {
	return []proto.Handler{
		proto.WithPackage(sw.Package),
		proto.WithRPC(sw.RPC),
		proto.WithMessage(sw.Message),
		proto.WithEnum(sw.Enum),
		proto.WithService(sw.Service),
		proto.WithImport(sw.Import),
	}
}

func (sw *Writer) Save(filename string) error {
	body := sw.Get()
	return os.WriteFile(filename, body, os.ModePerm^0111)
}

func (sw *Writer) Get() []byte {
	b, _ := json.MarshalIndent(sw, "", "  ")
	return b
}

func (sw *Writer) WalkFile() error {
	definition, err := sw.loadProtoFile(sw.filename)
	if err != nil {
		return err
	}

	// main file for all the relevant info
	proto.Walk(definition, sw.Handlers()...)

	if len(sw.Swagger.Paths.Paths) == 0 {
		return ErrNoServiceDefinition
	}
	return nil
}

func (sw *Writer) loadProtoFile(filename string) (*proto.Proto, error) {
	reader, err := os.Open(filepath.Join(sw.protoDir, filename))
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	parser := proto.NewParser(reader)
	return parser.Parse()
}
