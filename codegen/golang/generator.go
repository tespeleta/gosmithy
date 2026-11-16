// Package golang implements the Go code generation target: data types,
// enums, unions, errors, and validation methods.
package golang

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/tespeleta/gosmithy/codegen"
	"github.com/tespeleta/gosmithy/idl"
	"github.com/tespeleta/gosmithy/internal/runtime"
	"github.com/tespeleta/gosmithy/model"
	"github.com/tespeleta/gosmithy/model/httpbinding"
	"github.com/tespeleta/gosmithy/protocol/httpjson"
	"github.com/tespeleta/gosmithy/validation"
)

const (
	// Namespace is the Go generator namespace.
	Namespace = "go"
	// GeneratorName is the legacy combined Go option name.
	GeneratorName = Namespace
	// OptionModel emits the base Go module: types, validators, and codecs.
	OptionModel = "model"
	// OptionServer emits the base module plus the HTTP server side.
	OptionServer = "server"
	// OptionClient emits the base module plus the HTTP client side.
	OptionClient = "client"
)

type generateMode int

const (
	modeBase generateMode = iota
	modeFull
	modeServer
	modeClient
)

// New returns the Go generator. With an explicit protocol and no split HTTP
// generators selected it emits types, client, and server; otherwise it emits
// the model-only base.
func New() codegen.Generator { return &generator{mode: modeFull, name: GeneratorName} }

// NewModel returns the base Go generator: types, validators, and codecs.
func NewModel() codegen.Generator {
	return &generator{mode: modeBase, name: Namespace + ":" + OptionModel}
}

// NewServer returns a generator that emits the base module plus the HTTP
// server files.
func NewServer() codegen.Generator {
	return &generator{mode: modeServer, name: Namespace + ":" + OptionServer}
}

// NewClient returns a generator that emits the base module plus the HTTP
// client files.
func NewClient() codegen.Generator {
	return &generator{mode: modeClient, name: Namespace + ":" + OptionClient}
}

// Name implements codegen.NamedGenerator so the runner can identify the Go
// generator in error messages.
func (g *generator) Name() string {
	if g.name != "" {
		return g.name
	}
	return GeneratorName
}

// generator holds the per-request state of one Go generation run.
type generator struct {
	mode    generateMode
	name    string
	req     *codegen.Request
	m       *model.Model
	service *model.Shape
	module  string

	// shapes are the data shapes in the service closure, sorted by shape ID.
	shapes []*model.Shape

	// typeNames maps generated aggregate/enum shapes to Go type names.
	typeNames map[model.ShapeID]string
	// fieldNames maps member shape IDs to generated struct field names.
	fieldNames map[model.ShapeID]string
	// packageNames tracks package-level names to detect collisions.
	packageNames map[string]model.ShapeID
	// constantNames tracks generated enum constant names.
	constantNames map[string]model.ShapeID
	// unionWrappers maps union member shape IDs to wrapper struct names.
	unionWrappers map[model.ShapeID]string

	diagnostics []codegen.Diagnostic

	// patterns maps distinct @pattern values to package-level variable names.
	patterns     map[string]string
	patternOrder []string

	// Protocol generation state. bindings is empty for model-only requests.
	protocol       *codegen.Protocol
	bindings       []*httpbinding.Binding
	operationNames map[model.ShapeID]string
	rootPackage    string
	localCounter   int

	// inputShapes is the closure of every operation input. Relaxed decoding is
	// applied only to members of these shapes.
	inputShapes map[model.ShapeID]bool
	// relaxed maps member shape IDs to their effective relaxed-input setting.
	relaxed map[model.ShapeID]bool

	needsDecimal        bool
	needsRelaxedBool    bool
	needsRelaxedNumbers bool
	needsOptional       bool

	// needsBigIntRange and needsBigDecimalRange record generated @range checks
	// that require package-level comparison helpers.
	needsBigIntRange     bool
	needsBigDecimalRange bool

	useOptionals bool

	// goMod reports whether the base generator writes go.mod. Most projects
	// generate into an existing module, so this is opt-in.
	goMod bool
}

// Generate implements codegen.Generator.
func (g *generator) Generate(ctx context.Context, req *codegen.Request) (*codegen.Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req == nil || req.Model == nil {
		return nil, fmt.Errorf("golang: request has no model")
	}
	if req.Service.IsZero() {
		return nil, fmt.Errorf("golang: request has no service")
	}
	service, ok := req.Model.Shape(req.Service)
	if !ok {
		return nil, fmt.Errorf("golang: service `%s` is not found in the model", req.Service)
	}
	if service.Type != model.TypeService {
		return nil, fmt.Errorf("golang: `%s` is a %s shape, not a service", req.Service, service.Type)
	}

	mode := g.mode
	if mode == modeFull {
		if req.Protocol == nil || hasSplitHTTPGenerator(req.Target.Generators) {
			mode = modeBase
		}
	}
	g.mode = mode
	g.req = req
	g.m = req.Model
	g.service = service
	g.useOptionals = true
	if raw, ok := req.Options["useOptionals"]; ok {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("golang: invalid useOptionals value %q", raw)
		}
		g.useOptionals = value
	}
	if raw, ok := req.Options["goMod"]; ok {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("golang: invalid goMod value %q", raw)
		}
		g.goMod = value
	}
	emitBase := g.emitBase()
	emitProtocolRuntime := g.emitProtocolRuntime()
	g.typeNames = make(map[model.ShapeID]string)
	g.fieldNames = make(map[model.ShapeID]string)
	g.packageNames = make(map[string]model.ShapeID)
	g.constantNames = make(map[string]model.ShapeID)
	g.unionWrappers = make(map[model.ShapeID]string)
	g.patterns = make(map[string]string)

	g.module = strings.TrimRight(strings.TrimSpace(req.Options["module"]), "/")
	if g.module == "" {
		return nil, fmt.Errorf("golang: the %q option is required (set --module)", "module")
	}
	if strings.ContainsAny(g.module, " \t\r\n") {
		return nil, fmt.Errorf("golang: invalid module path %q", g.module)
	}
	g.checkOptions()

	closure, ok := g.m.ServiceClosure(service.ID)
	if !ok {
		return nil, fmt.Errorf("golang: cannot compute the closure of service `%s`", service.ID)
	}
	g.selectShapes(closure)
	g.buildTypeNames()
	g.buildFieldNames()
	g.computeRelaxedInput(closure)
	g.computeOptionalNeeds()
	g.reserveGeneratedTypes()

	if mode != modeBase {
		if req.Protocol == nil {
			g.diagnosticAt(validation.Danger, service.Source,
				"the Go HTTP generators require a protocol; the CLI supplies %s", httpjson.TraitID)
		} else {
			g.protocol = req.Protocol
			g.rootPackage = goPackageName(g.module)
			if !httpjson.Is(req.Protocol.ID) {
				g.diagnosticAt(validation.Danger, service.Source,
					"Unsupported protocol `%s`; the Go generator currently implements %s", req.Protocol.ID, httpjson.TraitID)
			} else {
				bindings, events := httpbinding.Index(g.m, service.ID)
				g.bindings = bindings
				for _, event := range events {
					g.diagnostics = append(g.diagnostics, codegen.Diagnostic{
						Severity: event.Severity,
						Message:  event.Message,
						Path:     event.Source.Filename,
						Line:     event.Source.Line,
						Column:   event.Source.Column,
					})
				}
				g.buildOperationNames()
				g.checkServiceProtocol()
			}
		}
	}

	files := make([]codegen.File, 0, 12)
	if emitBase && g.goMod {
		files = append(files, codegen.File{
			Path:    "go.mod",
			Content: []byte(fmt.Sprintf("module %s\n\ngo %s\n", g.module, g.goVersion())),
		})
	}

	runtimeFiles, err := runtime.FilesFor(mode != modeBase)
	if err != nil {
		return nil, err
	}
	if g.needsRelaxedNumbers && emitBase {
		relaxedFiles, err := runtime.RelaxedFiles()
		if err != nil {
			return nil, err
		}
		for path, content := range relaxedFiles {
			runtimeFiles[path] = content
		}
	}
	runtimePaths := make([]string, 0, len(runtimeFiles))
	for path := range runtimeFiles {
		if !emitBase && path != "internal/smithy/http.go" {
			continue
		}
		if !emitProtocolRuntime && path == "internal/smithy/http.go" {
			continue
		}
		runtimePaths = append(runtimePaths, path)
	}
	sort.Strings(runtimePaths)
	for _, path := range runtimePaths {
		content := append([]byte("// Code generated by gosmithy. DO NOT EDIT.\n\n"), runtimeFiles[path]...)
		files = append(files, codegen.File{Path: path, Content: content})
	}

	if emitBase {
		typesFile, err := g.emitTypesFile()
		if err != nil {
			return nil, fmt.Errorf("golang: generating types/types.go: %w", err)
		}
		files = append(files, codegen.File{Path: "types/types.go", Content: typesFile})

		if g.needsDecimal {
			decimalFile, err := g.emitDecimalFile()
			if err != nil {
				return nil, fmt.Errorf("golang: generating types/decimal.go: %w", err)
			}
			files = append(files, codegen.File{Path: "types/decimal.go", Content: decimalFile})
		}
		if g.needsRelaxedBool {
			relaxedFile, err := g.emitRelaxedBoolFile()
			if err != nil {
				return nil, fmt.Errorf("golang: generating types/relaxed.go: %w", err)
			}
			files = append(files, codegen.File{Path: "types/relaxed.go", Content: relaxedFile})
		}
		if g.needsOptional {
			optionalFile, err := g.emitOptionalFile()
			if err != nil {
				return nil, fmt.Errorf("golang: generating types/optional.go: %w", err)
			}
			files = append(files, codegen.File{Path: "types/optional.go", Content: optionalFile})
		}

		enumsFile, err := g.emitEnumsFile()
		if err != nil {
			return nil, fmt.Errorf("golang: generating types/enums.go: %w", err)
		}
		files = append(files, codegen.File{Path: "types/enums.go", Content: enumsFile})

		unionsFile, err := g.emitUnionsFile()
		if err != nil {
			return nil, fmt.Errorf("golang: generating types/unions.go: %w", err)
		}
		files = append(files, codegen.File{Path: "types/unions.go", Content: unionsFile})

		validateFile, err := g.emitValidateFile()
		if err != nil {
			return nil, fmt.Errorf("golang: generating types/validate.go: %w", err)
		}
		files = append(files, codegen.File{Path: "types/validate.go", Content: validateFile})

		jsonFile, err := g.emitJSONFile()
		if err != nil {
			return nil, fmt.Errorf("golang: generating types/json.go: %w", err)
		}
		if jsonFile != nil {
			files = append(files, codegen.File{Path: "types/json.go", Content: jsonFile})
		}
	}

	if g.protocol != nil && httpjson.Is(g.protocol.ID) {
		protocolFiles := make([]struct {
			path string
			emit func() ([]byte, error)
		}, 0, 4)
		if mode == modeFull || mode == modeServer {
			protocolFiles = append(protocolFiles, struct {
				path string
				emit func() ([]byte, error)
			}{"service.go", g.emitServiceFile})
		}
		if emitProtocolRuntime {
			protocolFiles = append(protocolFiles, struct {
				path string
				emit func() ([]byte, error)
			}{"protocol.go", g.emitProtocolFile})
		}
		if mode == modeFull || mode == modeClient {
			protocolFiles = append(protocolFiles, struct {
				path string
				emit func() ([]byte, error)
			}{"client.go", g.emitClientFile})
		}
		if mode == modeFull || mode == modeServer {
			protocolFiles = append(protocolFiles, struct {
				path string
				emit func() ([]byte, error)
			}{"server.go", g.emitServerFile})
		}
		for _, item := range protocolFiles {
			content, err := item.emit()
			if err != nil {
				return nil, fmt.Errorf("golang: generating %s: %w", item.path, err)
			}
			files = append(files, codegen.File{Path: item.path, Content: content})
		}
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return &codegen.Response{Files: files, Diagnostics: g.diagnostics}, nil
}

func (g *generator) hasGenerator(name string) bool {
	for _, selected := range g.req.Target.Generators {
		if selected == name {
			return true
		}
	}
	return false
}

func (g *generator) hasOption(option string) bool {
	return g.hasGenerator(Namespace + ":" + option)
}

// emitBase reports whether this generator writes the shared base files. When
// several Go generators run together, one of them owns go.mod, types, and the
// base runtime to avoid duplicate writes.
func (g *generator) emitBase() bool {
	switch g.mode {
	case modeBase, modeFull:
		return true
	case modeServer:
		return !g.hasOption(OptionModel)
	case modeClient:
		return !g.hasOption(OptionModel) && !g.hasOption(OptionServer)
	default:
		return true
	}
}

// emitProtocolRuntime reports whether this generator writes protocol.go and
// internal/smithy/http.go. The server owns them when both sides run together.
func (g *generator) emitProtocolRuntime() bool {
	switch g.mode {
	case modeBase:
		return false
	case modeClient:
		return !g.hasOption(OptionServer)
	default:
		return true
	}
}

func hasSplitHTTPGenerator(names []string) bool {
	for _, name := range names {
		if name != GeneratorName {
			return true
		}
	}
	return false
}

func (g *generator) checkOptions() {
	if len(g.req.Options) == 0 {
		return
	}
	keys := make([]string, 0, len(g.req.Options))
	for key := range g.req.Options {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if key != "module" && key != "useOptionals" && key != "goMod" {
			g.diagnostic(validation.Warning, idl.SourceLocation{}, "unknown Go generator option %q", key)
		}
	}
}

// selectShapes chooses the data shapes that belong in the generated types
// package. Operation, resource, service, and member shapes are represented by
// their data shapes; prelude simple shapes map directly to Go types.
func (g *generator) selectShapes(closure []model.ShapeID) {
	selected := make(map[model.ShapeID]*model.Shape)
	for _, id := range closure {
		shape, ok := g.m.Shape(id)
		if !ok || shape.IsMember() || shape.IsMixin() || shape.IsTraitDefinition() {
			continue
		}
		if g.isDataShape(shape) {
			selected[shape.ID] = shape
		}
	}
	// Unit is always emitted so generated operation signatures can use
	// types.Unit even when the model does not reference it explicitly.
	if unit, ok := g.m.Shape(model.TraitUnit); ok && unit.Type == model.TypeStructure {
		selected[unit.ID] = unit
	}

	g.shapes = make([]*model.Shape, 0, len(selected))
	for _, shape := range selected {
		g.shapes = append(g.shapes, shape)
	}
	sort.Slice(g.shapes, func(i, j int) bool { return g.shapes[i].ID.String() < g.shapes[j].ID.String() })
}

// buildOperationNames assigns the Go method name for every operation in the
// service closure and reports collisions. It is used by the protocol emitters.
func (g *generator) buildOperationNames() {
	g.operationNames = make(map[model.ShapeID]string)
	used := make(map[string]model.ShapeID)
	for _, bundle := range g.bindings {
		op, ok := g.m.Shape(bundle.Operation)
		if !ok || op.Type != model.TypeOperation {
			continue
		}
		name := safeTypeName(g.rawShapeName(op.ID))
		if previous, exists := used[name]; exists && previous != op.ID {
			g.diagnosticAt(validation.Danger, op.Source,
				"Go operation method name %q for `%s` collides with `%s`; use the service `rename` map to disambiguate",
				name, op.ID, previous)
		}
		used[name] = op.ID
		g.operationNames[op.ID] = name
	}
}

func (g *generator) operationGoName(op *model.Shape) string {
	if name, ok := g.operationNames[op.ID]; ok {
		return name
	}
	return safeTypeName(g.rawShapeName(op.ID))
}

func (g *generator) isDataShape(shape *model.Shape) bool {
	switch shape.Type {
	case model.TypeStructure, model.TypeUnion, model.TypeEnum, model.TypeIntEnum,
		model.TypeList, model.TypeSet, model.TypeMap:
		return true
	case model.TypeString:
		return shape.HasTrait(model.TraitEnum)
	default:
		return false
	}
}

func (g *generator) buildTypeNames() {
	// User-defined shapes claim names first so that a synthetic shape can
	// always be renamed out of the way.
	assign := func(shape *model.Shape, allowRename bool) {
		name := safeTypeName(g.rawShapeName(shape.ID))
		if previous, exists := g.packageNames[name]; exists && previous != shape.ID {
			if allowRename {
				// Synthetic shapes cannot be referenced by user code, so
				// renaming one is always safe and keeps generated code
				// compiling when a user shape owns the preferred name.
				name = g.freeSyntheticTypeName(name)
			} else {
				g.diagnosticAt(validation.Danger, shape.Source,
					"Go type name %q for `%s` collides with `%s`; use the service `rename` map to disambiguate",
					name, shape.ID, previous)
			}
		}
		g.typeNames[shape.ID] = name
		if _, exists := g.packageNames[name]; !exists {
			g.packageNames[name] = shape.ID
		}
	}
	for _, shape := range g.shapes {
		if !shape.HasTrait(model.TraitSynthetic) {
			assign(shape, false)
		}
	}
	for _, shape := range g.shapes {
		if shape.HasTrait(model.TraitSynthetic) {
			assign(shape, true)
		}
	}
}

// freeSyntheticTypeName returns name with a numeric suffix that is not already
// taken by another generated type.
func (g *generator) freeSyntheticTypeName(name string) string {
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s%d", name, i)
		if _, exists := g.packageNames[candidate]; !exists {
			return candidate
		}
	}
}

func (g *generator) buildFieldNames() {
	for _, shape := range g.shapes {
		if shape.Type != model.TypeStructure {
			continue
		}
		used := make(map[string]model.ShapeID)
		for _, member := range shape.Members {
			name := safeFieldName(g.rawMemberName(member))
			if previous, exists := used[name]; exists {
				g.diagnosticAt(validation.Danger, member.Source,
					"Go field name %q for `%s` collides with `%s`; use the service `rename` map to disambiguate",
					name, member.ID, previous)
				for i := 2; ; i++ {
					candidate := fmt.Sprintf("%s%d", name, i)
					if _, used := used[candidate]; !used {
						name = candidate
						break
					}
				}
			}
			used[name] = member.ID
			g.fieldNames[member.ID] = name
		}
	}
}

// rawShapeName applies a service rename to a shape ID before casing.
func (g *generator) rawShapeName(id model.ShapeID) string {
	if g.service.Service != nil {
		if renamed, ok := g.service.Service.Rename[id]; ok && renamed != "" {
			return renamed
		}
	}
	return id.Name
}

// rawMemberName applies a service rename to a member ID before casing.
func (g *generator) rawMemberName(member *model.Shape) string {
	if g.service.Service != nil {
		if renamed, ok := g.service.Service.Rename[member.ID]; ok && renamed != "" {
			return renamed
		}
	}
	return member.ID.Member
}

func (g *generator) typeName(shape *model.Shape) (string, bool) {
	name, ok := g.typeNames[shape.ID]
	return name, ok
}

func (g *generator) fieldName(member *model.Shape) string {
	if name, ok := g.fieldNames[member.ID]; ok {
		return name
	}
	return safeFieldName(g.rawMemberName(member))
}

// memberJSONName returns the JSON object key for a member, honoring @jsonName.
func (g *generator) memberJSONName(member *model.Shape) string {
	if name, ok := member.JSONName(); ok && name != "" {
		return name
	}
	return member.ID.Member
}

func (g *generator) diagnostic(severity validation.Severity, loc idl.SourceLocation, format string, args ...any) {
	g.diagnostics = append(g.diagnostics, codegen.Diagnostic{
		Severity: severity,
		Message:  fmt.Sprintf(format, args...),
		Path:     loc.Filename,
		Line:     loc.Line,
		Column:   loc.Column,
	})
}

func (g *generator) diagnosticAt(severity validation.Severity, loc idl.SourceLocation, format string, args ...any) {
	g.diagnostic(severity, loc, format, args...)
}

// --- type mapping -------------------------------------------------------------

// valueType returns the Go type used for values of target. Pointers for
// optional member presence are added by memberFieldType and pointerized.
func (g *generator) valueType(target *model.Shape, w *fileWriter) (string, bool) {
	if target == nil {
		return "", false
	}
	switch target.Type {
	case model.TypeString:
		if name, ok := g.typeNames[target.ID]; ok {
			return name, true
		}
		return "string", true
	case model.TypeBlob:
		return "[]byte", true
	case model.TypeBoolean:
		return "bool", true
	case model.TypeByte:
		return "int8", true
	case model.TypeShort:
		return "int16", true
	case model.TypeInteger:
		return "int32", true
	case model.TypeLong:
		return "int64", true
	case model.TypeFloat:
		return "float32", true
	case model.TypeDouble:
		return "float64", true
	case model.TypeBigInteger:
		w.AddImport("math/big", "")
		return "*big.Int", true
	case model.TypeBigDecimal:
		return "Decimal", true
	case model.TypeTimestamp:
		w.AddImport("time", "")
		return "time.Time", true
	case model.TypeDocument:
		return "any", true
	case model.TypeList, model.TypeSet, model.TypeMap, model.TypeStructure,
		model.TypeUnion, model.TypeEnum, model.TypeIntEnum:
		if name, ok := g.typeNames[target.ID]; ok {
			return name, true
		}
		if target.Type == model.TypeList || target.Type == model.TypeSet {
			elem, ok := g.collectionElementValueType(target, w)
			if !ok {
				return "", false
			}
			return "[]" + elem, true
		}
		if target.Type == model.TypeMap {
			value, ok := g.mapValueValueType(target, w)
			if !ok {
				return "", false
			}
			return "map[string]" + value, true
		}
		return "", false
	default:
		return "", false
	}
}

// memberFieldType returns the Go type of a structure field. It adds a pointer
// for shapes whose absence is represented by nil rather than a nilable Go
// value.
func (g *generator) memberFieldType(member *model.Shape, w *fileWriter) (string, bool) {
	target, ok := g.m.Shape(member.Target)
	if !ok {
		g.diagnosticAt(validation.Danger, member.Source,
			"Member `%s` targets `%s`, which is not found in the model", member.ID, member.Target)
		return "", false
	}
	typ, ok := g.valueType(target, w)
	if !ok {
		g.diagnosticAt(validation.Danger, member.Source,
			"Member `%s` targets `%s`, which cannot be represented as a Go type", member.ID, member.Target)
		return "", false
	}
	if g.optionalFor(member, target) {
		inner, ok := g.optionalInnerType(member, target, w)
		if !ok || inner == "" {
			inner = "any"
		}
		return "Optional[" + inner + "]", true
	}
	if g.fieldIsPlainValue(member, target) {
		inner, ok := g.plainInnerType(member, target, w)
		if !ok || inner == "" {
			inner = "any"
		}
		return inner, true
	}
	if target.Type == model.TypeBoolean && g.isRelaxed(member) {
		return "*RelaxedBool", true
	}
	if fieldHasPointerWrapper(target) {
		return "*" + typ, true
	}
	return typ, true
}

// fieldHasPointerWrapper reports whether a member field adds a pointer beyond
// the Go representation of the target value.
func fieldHasPointerWrapper(target *model.Shape) bool {
	switch target.Type {
	case model.TypeBlob, model.TypeList, model.TypeSet, model.TypeMap,
		model.TypeDocument, model.TypeBigInteger, model.TypeUnion:
		return false
	default:
		return true
	}
}

// collectionElementValueType returns the Go type of a list/set element,
// including the pointer required by @sparse.
func (g *generator) collectionElementValueType(collection *model.Shape, w *fileWriter) (string, bool) {
	member, ok := collection.GetMember("member")
	if !ok {
		g.diagnosticAt(validation.Danger, collection.Source, "List `%s` has no member", collection.ID)
		return "", false
	}
	target, ok := g.m.Shape(member.Target)
	if !ok {
		g.diagnosticAt(validation.Danger, member.Source,
			"Member `%s` targets `%s`, which is not found in the model", member.ID, member.Target)
		return "", false
	}
	typ, ok := g.valueType(target, w)
	if !ok {
		g.diagnosticAt(validation.Danger, member.Source,
			"Member `%s` targets `%s`, which cannot be represented as a Go type", member.ID, member.Target)
		return "", false
	}
	if collection.IsSparse() {
		return g.pointerize(typ, target, w), true
	}
	return typ, true
}

// mapValueValueType returns the Go type of a map value, including @sparse.
func (g *generator) mapValueValueType(m *model.Shape, w *fileWriter) (string, bool) {
	member, ok := m.GetMember("value")
	if !ok {
		g.diagnosticAt(validation.Danger, m.Source, "Map `%s` has no value member", m.ID)
		return "", false
	}
	target, ok := g.m.Shape(member.Target)
	if !ok {
		g.diagnosticAt(validation.Danger, member.Source,
			"Member `%s` targets `%s`, which is not found in the model", member.ID, member.Target)
		return "", false
	}
	typ, ok := g.valueType(target, w)
	if !ok {
		g.diagnosticAt(validation.Danger, member.Source,
			"Member `%s` targets `%s`, which cannot be represented as a Go type", member.ID, member.Target)
		return "", false
	}
	if m.IsSparse() {
		return g.pointerize(typ, target, w), true
	}
	return typ, true
}

// pointerize turns a Go value type into the representation used for a sparse
// collection element or a union member that permits nil.
func (g *generator) pointerize(typ string, target *model.Shape, w *fileWriter) string {
	switch target.Type {
	case model.TypeBlob, model.TypeDocument:
		return typ
	case model.TypeBigInteger:
		return typ
	case model.TypeUnion:
		// A union is a Go interface; a nil interface already represents an
		// absent sparse element.
		return typ
	case model.TypeList, model.TypeSet, model.TypeMap:
		if _, named := g.typeNames[target.ID]; named {
			return "*" + typ
		}
		return typ
	default:
		if strings.HasPrefix(typ, "*") {
			return typ
		}
		return "*" + typ
	}
}

// unionValueType returns the Go type stored in a union wrapper. Simple values
// are stored by value; structures, unions, and named collections are stored by
// pointer.
func (g *generator) unionValueType(member *model.Shape, w *fileWriter) (string, bool) {
	target, ok := g.m.Shape(member.Target)
	if !ok {
		g.diagnosticAt(validation.Danger, member.Source,
			"Member `%s` targets `%s`, which is not found in the model", member.ID, member.Target)
		return "", false
	}
	typ, ok := g.valueType(target, w)
	if !ok {
		g.diagnosticAt(validation.Danger, member.Source,
			"Member `%s` targets `%s`, which cannot be represented as a Go type", member.ID, member.Target)
		return "", false
	}
	if target.Type == model.TypeBoolean && g.isRelaxed(member) {
		return "RelaxedBool", true
	}
	switch target.Type {
	case model.TypeStructure, model.TypeList, model.TypeSet, model.TypeMap:
		return "*" + typ, true
	case model.TypeUnion:
		// A union is an interface; storing it directly is nilable.
		return typ, true
	default:
		return typ, true
	}
}

func (g *generator) hasGeneratedValidator(target *model.Shape) bool {
	if target == nil {
		return false
	}
	switch target.Type {
	case model.TypeStructure, model.TypeList, model.TypeSet, model.TypeMap:
		_, ok := g.typeNames[target.ID]
		return ok
	default:
		return false
	}
}

func (g *generator) isStringLike(target *model.Shape) bool {
	return target != nil && target.Type == model.TypeString
}
