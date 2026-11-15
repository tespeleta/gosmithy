package model

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/tespeleta/gosmithy/idl"
)

// Options controls model assembly.
type Options struct {
	// DisablePrelude assembles the model without the Smithy prelude. This is
	// only useful for tests and for inspecting a model in isolation.
	DisablePrelude bool
}

// Assemble turns parsed IDL files into a semantic model.
//
// The prelude is parsed and assembled first unless Options.DisablePrelude is
// set. Assembly never returns user errors: problems are recorded as
// diagnostics on the returned model. The only error is an internal failure to
// load the embedded prelude.
func Assemble(files []*idl.File, opts Options) (*Model, error) {
	a := &assembler{
		model:               NewModel(),
		declared:            make(map[ShapeID]bool),
		publicPrelude:       make(map[string]bool),
		preludePrivate:      make(map[string]bool),
		traitDefs:           make(map[ShapeID]bool),
		syntheticSignatures: make(map[ShapeID]string),
		flattenState:        make(map[ShapeID]flattenState),
	}
	a.traitDefs[TraitTrait] = true

	if !opts.DisablePrelude {
		prelude, err := ParsePrelude()
		if err != nil {
			return nil, err
		}
		a.files = append(a.files, newFileData(prelude, true))
	}

	sorted := make([]*idl.File, 0, len(files))
	for _, f := range files {
		if f != nil {
			sorted = append(sorted, f)
		}
	}
	// The built-in protocol-trait definitions make 2.1 member indexes work
	// without the user supplying smithy.protocols#idx. A user definition of
	// the same shape wins and suppresses the built-in one.
	if !opts.DisablePrelude && !definesProtocolsIdx(files) {
		builtin, err := parseBuiltinTraits()
		if err != nil {
			return nil, err
		}
		sorted = append(sorted, builtin)
	}
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	for _, f := range sorted {
		a.files = append(a.files, newFileData(f, false))
	}

	a.scanPrelude()
	a.buildSkeletons()
	a.collectTraitDefinitions()
	a.applyShapeTraits()
	a.flattenAll()
	a.applyRemainingTraits()
	a.mergeMetadata()
	a.resolveUses()

	a.model.diagnostics = append(a.model.diagnostics, a.diags...)
	sortDiagnostics(a.model.diagnostics)
	return a.model, nil
}

type fileData struct {
	file      *idl.File
	namespace string
	version   idl.Version
	uses      map[string]ShapeID
	prelude   bool
}

func newFileData(f *idl.File, prelude bool) *fileData {
	fd := &fileData{
		file:      f,
		namespace: f.Namespace,
		version:   f.Version,
		uses:      make(map[string]ShapeID),
		prelude:   prelude,
	}
	for _, u := range f.Uses {
		if id, err := ParseShapeID(u.ID); err == nil {
			fd.uses[id.Name] = id
		}
	}
	return fd
}

type pendingTrait struct {
	target     ShapeID
	name       string // raw trait name; empty for documentation comments
	value      idl.Node
	annotation bool
	source     idl.SourceLocation
	fd         *fileData
	fromApply  bool
	applied    bool
}

type flattenState int

const (
	flattenUnvisited flattenState = iota
	flattenVisiting
	flattenDone
)

type assembler struct {
	model     *Model
	files     []*fileData
	diags     []Diagnostic
	pending   []*pendingTrait
	traitDefs map[ShapeID]bool
	// caseIndex maps a lower-cased shape ID string to the first declared ID,
	// used to detect case-insensitive conflicts.
	caseIndex map[string]ShapeID
	// declared holds every root shape ID declared by a file, including forward
	// references, so local names shadow the prelude regardless of file order.
	declared map[ShapeID]bool
	// publicPrelude and preludePrivate map prelude root names to visibility.
	publicPrelude  map[string]bool
	preludePrivate map[string]bool

	// syntheticSignatures records the content signature of every inline
	// collection shape created by the assembler, keyed by shape ID.
	syntheticSignatures map[ShapeID]string

	flattenState map[ShapeID]flattenState
}

func (a *assembler) danger(id string, loc idl.SourceLocation, shapeID ShapeID, format string, args ...any) {
	a.diags = append(a.diags, Diagnostic{
		ID:       id,
		Severity: SeverityDanger,
		Message:  fmt.Sprintf(format, args...),
		ShapeID:  shapeID,
		Source:   loc,
	})
}

// scanPrelude determines which prelude shapes are public and which are private
// so relative IDs can resolve to public prelude shapes only. It also records
// every declared root shape ID so that local forward references shadow the
// prelude, matching the reference implementation.
func (a *assembler) scanPrelude() {
	if a.caseIndex == nil {
		a.caseIndex = make(map[string]ShapeID)
	}
	for _, fd := range a.files {
		for _, stmt := range fd.file.Shapes {
			if s, ok := stmt.(*idl.ShapeStmt); ok {
				if id, err := ParseShapeID(s.ID); err == nil {
					a.declared[id] = true
				}
			}
			if !fd.prelude {
				continue
			}
			s, ok := stmt.(*idl.ShapeStmt)
			if !ok {
				continue
			}
			private := false
			for _, tr := range s.Traits {
				if tr.Name == "private" || tr.Name == "smithy.api#private" {
					private = true
				}
			}
			if private {
				a.preludePrivate[s.Name] = true
			} else {
				a.publicPrelude[s.Name] = true
			}
		}
	}
}

// --- phase B: build skeletons -------------------------------------------------

func (a *assembler) buildSkeletons() {
	for _, fd := range a.files {
		for _, stmt := range fd.file.Shapes {
			switch s := stmt.(type) {
			case *idl.ShapeStmt:
				a.declareShape(s, fd)
			case *idl.ApplyStmt:
				a.addApply(s, fd)
			}
		}
	}
}

func (a *assembler) declareShape(stmt *idl.ShapeStmt, fd *fileData) {
	id, err := ParseShapeID(stmt.ID)
	if err != nil {
		a.danger("Model.DuplicateShapeId", stmt.Source, ShapeID{}, "Invalid shape ID: %v", err)
		return
	}
	typ := shapeTypeFromKind(stmt.Kind)

	shape, exists := a.model.Shape(id)
	if !exists {
		if first, ok := a.caseIndex[strings.ToLower(id.String())]; ok && first != id {
			a.danger("Model.DuplicateShapeId", stmt.Source, id,
				"Shape ID `%s` conflicts with `%s` (shape IDs are case-insensitive)", id, first)
		}
		a.caseIndex[strings.ToLower(id.String())] = id

		shape = NewShape(id, typ)
		shape.Source = stmt.Source
		shape.Doc = stmt.Doc
		shape.prelude = fd.prelude
		shape.version = fd.version
		a.model.addShape(shape)
		a.checkShapeVersion(shape, stmt, fd)
	} else if shape.Type != typ {
		a.danger("Model.DuplicateShapeId", stmt.Source, id,
			"Cannot redefine shape `%s` as `%s` because it is already defined as `%s`", id, typ, shape.Type)
		return
	}

	if shape.Doc == "" {
		shape.Doc = stmt.Doc
	}
	a.addTraits(id, stmt.Traits, fd)
	if stmt.Doc != "" {
		a.addDoc(id, stmt.Doc, stmt.DocSource, fd)
	}

	for _, mixin := range stmt.Mixins {
		shape.Mixins = appendUniqueRefs(shape.Mixins, []ShapeID{a.resolveRef(fd, mixin.ID)})
	}
	if stmt.ForResource != "" {
		if shape.ForResource.IsZero() {
			shape.ForResource = a.resolveRef(fd, stmt.ForResource)
		}
	}

	switch typ {
	case TypeList, TypeSet, TypeMap, TypeStructure, TypeUnion:
		a.declareMembers(shape, stmt.Members, fd)
	case TypeEnum, TypeIntEnum:
		a.declareEnumMembers(shape, stmt.Members, fd)
	case TypeService:
		if stmt.Body != nil {
			a.declareService(shape, stmt.Body, fd)
		}
	case TypeResource:
		if stmt.Body != nil {
			a.declareResource(shape, stmt.Body, fd)
		}
	case TypeOperation:
		a.declareOperation(shape, stmt, fd)
	}
}

func shapeTypeFromKind(kind idl.ShapeKind) ShapeType {
	switch kind {
	case idl.KindBlob:
		return TypeBlob
	case idl.KindBoolean:
		return TypeBoolean
	case idl.KindDocument:
		return TypeDocument
	case idl.KindString:
		return TypeString
	case idl.KindByte:
		return TypeByte
	case idl.KindShort:
		return TypeShort
	case idl.KindInteger:
		return TypeInteger
	case idl.KindLong:
		return TypeLong
	case idl.KindFloat:
		return TypeFloat
	case idl.KindDouble:
		return TypeDouble
	case idl.KindBigInteger:
		return TypeBigInteger
	case idl.KindBigDecimal:
		return TypeBigDecimal
	case idl.KindTimestamp:
		return TypeTimestamp
	case idl.KindEnum:
		return TypeEnum
	case idl.KindIntEnum:
		return TypeIntEnum
	case idl.KindList:
		return TypeList
	case idl.KindSet:
		return TypeSet
	case idl.KindMap:
		return TypeMap
	case idl.KindStructure:
		return TypeStructure
	case idl.KindUnion:
		return TypeUnion
	case idl.KindService:
		return TypeService
	case idl.KindResource:
		return TypeResource
	case idl.KindOperation:
		return TypeOperation
	default:
		return ShapeType(kind)
	}
}

func (a *assembler) checkShapeVersion(shape *Shape, stmt *idl.ShapeStmt, fd *fileData) {
	switch shape.Type {
	case TypeSet:
		if fd.version.AtLeast(idl.V2_0) {
			a.danger("Model.InvalidShapeType", stmt.Source, shape.ID,
				"set shapes cannot be used in Smithy version %s", fd.version)
		}
	case TypeEnum:
		if !fd.version.AtLeast(idl.V2_0) {
			a.danger("Model.InvalidEnum", stmt.Source, shape.ID,
				"enum shapes cannot be used in Smithy version %s", fd.version)
		}
	case TypeIntEnum:
		if !fd.version.AtLeast(idl.V2_0) {
			a.danger("Model.InvalidEnum", stmt.Source, shape.ID,
				"intEnum shapes cannot be used in Smithy version %s", fd.version)
		}
	}
}

func (a *assembler) declareMembers(shape *Shape, members []idl.Member, fd *fileData) {
	for _, m := range members {
		a.declareMember(shape, m, fd)
	}
}

func (a *assembler) declareMember(shape *Shape, m idl.Member, fd *fileData) {
	var target ShapeID
	if !m.Elided {
		if m.Target.IsInline() {
			target = a.declareInlineCollection(shape.ID.Namespace, m.Target, fd)
		} else {
			target = a.resolveRef(fd, m.Target.ID)
		}
	}

	if existing, ok := shape.GetMember(m.Name); ok {
		if !target.IsZero() && !existing.Target.IsZero() && existing.Target != target {
			a.danger("Model.InvalidMember", m.Source, existing.ID,
				"Member `%s` conflicts with an earlier definition targeting `%s`", existing.ID, existing.Target)
		}
		if existing.Target.IsZero() && !target.IsZero() {
			existing.Target = target
		}
		a.addTraits(existing.ID, m.Traits, fd)
		a.addMemberDefault(existing, m, fd)
		a.addMemberIndex(existing.ID, m, fd)
		if m.Doc != "" {
			a.addDoc(existing.ID, m.Doc, m.DocSource, fd)
		}
		return
	}

	member := NewShape(shape.ID.WithMember(m.Name), TypeMember)
	member.Container = shape.ID
	member.Target = target
	member.Source = m.Source
	member.Doc = m.Doc
	member.Elided = m.Elided
	member.prelude = shape.prelude
	member.version = fd.version
	shape.AddMember(member)
	a.model.addShape(member)

	a.addTraits(member.ID, m.Traits, fd)
	a.addMemberDefault(member, m, fd)
	a.addMemberIndex(member.ID, m, fd)
	if m.Doc != "" {
		a.addDoc(member.ID, m.Doc, m.DocSource, fd)
	}
}

// addMemberIndex applies the smithy.protocols#idx trait for 2.1 member index
// shorthand. Members are not reordered, and no range or contiguity rules are
// enforced here; the trait only records the declared index.
func (a *assembler) addMemberIndex(memberID ShapeID, m idl.Member, fd *fileData) {
	if m.Index == nil {
		return
	}
	a.pending = append(a.pending, &pendingTrait{
		target: memberID,
		name:   "smithy.protocols#idx",
		value: idl.Node{
			Kind:   idl.NodeNumber,
			Num:    strconv.Itoa(*m.Index),
			Source: m.Source,
		},
		source: m.Source,
		fd:     fd,
	})
}

func (a *assembler) addMemberDefault(member *Shape, m idl.Member, fd *fileData) {
	if m.Value == nil {
		return
	}
	value := nodeFromIDL(*m.Value, func(raw string) ShapeID { return a.resolveSyntactic(fd, raw) })
	a.setTrait(member, &Trait{ID: TraitDefault, Value: value, Source: m.Value.Source})
}

func (a *assembler) declareEnumMembers(shape *Shape, members []idl.Member, fd *fileData) {
	for _, m := range members {
		if existing, ok := shape.GetMember(m.Name); ok {
			a.addTraits(existing.ID, m.Traits, fd)
			continue
		}
		member := NewShape(shape.ID.WithMember(m.Name), TypeMember)
		member.Container = shape.ID
		member.Target = TraitUnit
		member.Source = m.Source
		member.Doc = m.Doc
		member.prelude = shape.prelude
		member.version = fd.version
		shape.AddMember(member)
		a.model.addShape(member)

		a.addTraits(member.ID, m.Traits, fd)
		if m.Doc != "" {
			a.addDoc(member.ID, m.Doc, m.DocSource, fd)
		}

		if m.Value != nil {
			value := nodeFromIDL(*m.Value, func(raw string) ShapeID { return a.resolveSyntactic(fd, raw) })
			a.setTrait(member, &Trait{ID: TraitEnumValue, Value: value, Source: m.Value.Source})
		} else if shape.Type == TypeEnum && !hasTraitNamed(a, fd, m.Traits, TraitEnumValue) {
			a.setTrait(member, &Trait{
				ID:     TraitEnumValue,
				Value:  StringNode(m.Name),
				Source: m.Source,
			})
		}
	}
}

func (a *assembler) declareService(shape *Shape, body *idl.Node, fd *fileData) {
	node := nodeFromIDL(*body, func(raw string) ShapeID { return a.resolveSyntactic(fd, raw) })
	if shape.Service == nil {
		shape.Service = &ServiceProps{}
	}
	props := shape.Service

	if v, ok := node.Get("version"); ok && v.Kind == NodeString {
		if props.Version == "" {
			props.Version = v.Str
		} else if props.Version != v.Str {
			a.danger("Model.Conflict", v.Source, shape.ID,
				"Service `%s` has conflicting version values: `%s` and `%s`", shape.ID, props.Version, v.Str)
		}
	}
	props.Operations = appendUniqueRefs(props.Operations, a.refsFromNode(fd, node, "operations"))
	props.Resources = appendUniqueRefs(props.Resources, a.refsFromNode(fd, node, "resources"))
	props.Errors = appendUniqueRefs(props.Errors, a.refsFromNode(fd, node, "errors"))

	if v, ok := node.Get("rename"); ok && v.Kind == NodeObject {
		if props.Rename == nil {
			props.Rename = make(map[ShapeID]string)
		}
		keys := append([]ObjectEntry(nil), v.Obj...)
		sort.SliceStable(keys, func(i, j int) bool { return keys[i].Key < keys[j].Key })
		for _, e := range keys {
			id := a.parseRef(fd, e.Key)
			if id.IsZero() || e.Value.Kind != NodeString {
				continue
			}
			if previous, ok := props.Rename[id]; ok && previous != e.Value.Str {
				a.danger("Model.Conflict", e.Value.Source, shape.ID,
					"Service `%s` has conflicting rename values for `%s`", shape.ID, id)
				continue
			}
			props.Rename[id] = e.Value.Str
		}
	}
}

func (a *assembler) declareResource(shape *Shape, body *idl.Node, fd *fileData) {
	node := nodeFromIDL(*body, func(raw string) ShapeID { return a.resolveSyntactic(fd, raw) })
	if shape.Resource == nil {
		shape.Resource = &ResourceProps{}
	}
	props := shape.Resource

	if props.Identifiers == nil {
		props.Identifiers = make(map[string]ShapeID)
	}
	if props.Properties == nil {
		props.Properties = make(map[string]ShapeID)
	}
	mergeRefMap(a, shape, props.Identifiers, node, "identifiers", fd)
	mergeRefMap(a, shape, props.Properties, node, "properties", fd)

	setLifecycle := func(name string, dst *ShapeID) {
		if v, ok := node.Get(name); ok {
			id := a.shapeIDFromNode(fd, v)
			if id.IsZero() {
				return
			}
			if !dst.IsZero() && *dst != id {
				a.danger("Model.Conflict", v.Source, shape.ID,
					"Resource `%s` has conflicting `%s` values: `%s` and `%s`", shape.ID, name, *dst, id)
				return
			}
			*dst = id
		}
	}
	setLifecycle("create", &props.Create)
	setLifecycle("read", &props.Read)
	setLifecycle("update", &props.Update)
	setLifecycle("delete", &props.Delete)
	setLifecycle("list", &props.List)
	setLifecycle("put", &props.Put)

	props.Operations = appendUniqueRefs(props.Operations, a.refsFromNode(fd, node, "operations"))
	props.CollectionOperations = appendUniqueRefs(props.CollectionOperations, a.refsFromNode(fd, node, "collectionOperations"))
	props.Resources = appendUniqueRefs(props.Resources, a.refsFromNode(fd, node, "resources"))
}

func mergeRefMap(a *assembler, shape *Shape, dst map[string]ShapeID, node Node, key string, fd *fileData) {
	v, ok := node.Get(key)
	if !ok || v.Kind != NodeObject {
		return
	}
	entries := append([]ObjectEntry(nil), v.Obj...)
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })
	for _, e := range entries {
		id := a.shapeIDFromNode(fd, e.Value)
		if id.IsZero() {
			continue
		}
		if previous, ok := dst[e.Key]; ok && previous != id {
			a.danger("Model.Conflict", e.Value.Source, shape.ID,
				"Resource `%s` has conflicting `%s` values for `%s`: `%s` and `%s`",
				shape.ID, key, e.Key, previous, id)
			continue
		}
		dst[e.Key] = id
	}
}

func (a *assembler) declareOperation(shape *Shape, stmt *idl.ShapeStmt, fd *fileData) {
	if !shape.Input.IsZero() || !shape.Output.IsZero() {
		// A duplicate operation definition; validate that the properties agree.
		a.mergeOperation(shape, stmt, fd)
		return
	}

	shape.Input = a.operationIO(shape, stmt.Input, fd, "input", true)
	shape.Output = a.operationIO(shape, stmt.Output, fd, "output", false)

	for _, ref := range stmt.Errors {
		shape.Errors = append(shape.Errors, a.resolveRef(fd, ref.ID))
	}
}

func (a *assembler) mergeOperation(shape *Shape, stmt *idl.ShapeStmt, fd *fileData) {
	input := a.operationIO(shape, stmt.Input, fd, "input", true)
	output := a.operationIO(shape, stmt.Output, fd, "output", false)
	if !input.IsZero() && shape.Input != input {
		a.danger("Model.InvalidOperationIo", stmt.Source, shape.ID,
			"Operation `%s` has conflicting input shapes: `%s` and `%s`", shape.ID, shape.Input, input)
	}
	if !output.IsZero() && shape.Output != output {
		a.danger("Model.InvalidOperationIo", stmt.Source, shape.ID,
			"Operation `%s` has conflicting output shapes: `%s` and `%s`", shape.ID, shape.Output, output)
	}
	for _, ref := range stmt.Errors {
		shape.Errors = appendUniqueRefs(shape.Errors, []ShapeID{a.resolveRef(fd, ref.ID)})
	}
}

func (a *assembler) operationIO(shape *Shape, io *idl.InlineIO, fd *fileData, kind string, input bool) ShapeID {
	if io == nil {
		return TraitUnit
	}
	if io.Target != nil {
		return a.resolveRef(fd, io.Target.ID)
	}

	suffix := "Input"
	if !input {
		suffix = "Output"
	}
	if control, ok := controlString(fd.file, "operation"+strings.ToUpper(suffix[:1])+suffix[1:]+"Suffix"); ok {
		suffix = control
	}
	id := NewShapeID(fd.namespace, shape.ID.Name+suffix)

	synthetic, exists := a.model.Shape(id)
	if !exists {
		synthetic = NewShape(id, TypeStructure)
		synthetic.Source = shape.Source
		synthetic.Doc = io.Doc
		synthetic.prelude = fd.prelude
		synthetic.version = fd.version
		a.model.addShape(synthetic)
		a.caseIndex[strings.ToLower(id.String())] = id
	} else if synthetic.Type != TypeStructure {
		a.danger("Model.InvalidOperationIo", io.Source, shape.ID,
			"Operation `%s` has an inline %s that conflicts with non-structure shape `%s`", shape.ID, kind, id)
		return id
	}

	if synthetic.Doc == "" {
		synthetic.Doc = io.Doc
	}
	for _, mixin := range io.Mixins {
		synthetic.Mixins = appendUniqueRefs(synthetic.Mixins, []ShapeID{a.resolveRef(fd, mixin.ID)})
	}
	if io.ForResource != "" {
		synthetic.ForResource = a.resolveRef(fd, io.ForResource)
	}
	a.declareMembers(synthetic, io.Members, fd)
	a.addTraits(id, io.Traits, fd)
	traitName := "smithy.api#input"
	if !input {
		traitName = "smithy.api#output"
	}
	a.pending = append(a.pending, &pendingTrait{
		target:     id,
		name:       traitName,
		value:      idl.Node{Kind: idl.NodeNull, Source: io.Source},
		annotation: true,
		source:     io.Source,
		fd:         fd,
	})
	if io.Doc != "" {
		a.addDoc(id, io.Doc, io.DocSource, fd)
	}
	return id
}

func controlString(file *idl.File, name string) (string, bool) {
	for _, c := range file.Controls {
		if c.Name == name && c.Value.Kind == idl.NodeString {
			return c.Value.Str, true
		}
	}
	return "", false
}

func (a *assembler) addApply(stmt *idl.ApplyStmt, fd *fileData) {
	target := a.resolveRef(fd, stmt.Target.ID)
	for _, tr := range stmt.Traits {
		a.pending = append(a.pending, &pendingTrait{
			target:     target,
			name:       tr.Name,
			value:      tr.Value,
			annotation: tr.Annotation,
			source:     tr.Source,
			fd:         fd,
			fromApply:  true,
		})
	}
}

func (a *assembler) addTraits(target ShapeID, traits []idl.Trait, fd *fileData) {
	for _, tr := range traits {
		a.pending = append(a.pending, &pendingTrait{
			target:     target,
			name:       tr.Name,
			value:      tr.Value,
			annotation: tr.Annotation,
			source:     tr.Source,
			fd:         fd,
		})
	}
}

func (a *assembler) addDoc(target ShapeID, doc string, source idl.SourceLocation, fd *fileData) {
	if doc == "" {
		return
	}
	a.pending = append(a.pending, &pendingTrait{
		target: target,
		value:  idl.Node{Kind: idl.NodeString, Str: doc, Source: source},
		source: source,
		fd:     fd,
	})
}

// --- resolution ---------------------------------------------------------------

// resolveRef resolves a relative or absolute shape reference using the rules
// described by the Smithy IDL: use aliases, then the current namespace, then
// public prelude shapes, then the current namespace as a fallback (which will
// later be reported as unresolved).
func (a *assembler) resolveRef(fd *fileData, ref string) ShapeID {
	if ref == "" {
		return ShapeID{}
	}
	if strings.ContainsRune(ref, '#') {
		id, err := ParseShapeID(ref)
		if err != nil {
			return ShapeID{}
		}
		return id
	}

	root, member, _ := strings.Cut(ref, "$")
	var base ShapeID
	switch {
	case fd == nil || fd.namespace == "":
		base = NewShapeID(PreludeNamespace, root)
	case fd.uses[root] != (ShapeID{}):
		base = fd.uses[root]
	default:
		candidate := NewShapeID(fd.namespace, root)
		if a.declared[candidate] {
			base = candidate
		} else if a.publicPrelude[root] {
			base = NewShapeID(PreludeNamespace, root)
		} else {
			base = candidate
		}
	}
	if member != "" {
		base.Member = member
	}
	return base
}

// resolveSyntactic resolves an unquoted identifier used as a node value. The
// result is always a shape ID; unresolved IDs fall back to the current
// namespace (or smithy.api for metadata) and are reported by validation.
func (a *assembler) resolveSyntactic(fd *fileData, raw string) ShapeID {
	if raw == "" {
		return ShapeID{}
	}
	if strings.ContainsRune(raw, '#') {
		if id, err := ParseShapeID(raw); err == nil {
			return id
		}
	}
	root, member, _ := strings.Cut(raw, "$")
	namespace := PreludeNamespace
	if fd != nil && fd.namespace != "" {
		namespace = fd.namespace
	}
	var base ShapeID
	if fd != nil && fd.uses[root] != (ShapeID{}) {
		base = fd.uses[root]
	} else if fd != nil && fd.namespace != "" && a.declared[NewShapeID(fd.namespace, root)] {
		base = NewShapeID(fd.namespace, root)
	} else if a.publicPrelude[root] {
		base = NewShapeID(PreludeNamespace, root)
	} else {
		base = NewShapeID(namespace, root)
	}
	if member != "" {
		base.Member = member
	}
	return base
}

// parseRef resolves a string that must already be a shape ID (for example a
// rename map key).
func (a *assembler) parseRef(fd *fileData, ref string) ShapeID {
	if ref == "" {
		return ShapeID{}
	}
	if strings.ContainsRune(ref, '#') {
		id, err := ParseShapeID(ref)
		if err == nil {
			return id
		}
		return ShapeID{}
	}
	return a.resolveRef(fd, ref)
}

// --- phase C: traits ----------------------------------------------------------

func (a *assembler) collectTraitDefinitions() {
	for _, fd := range a.files {
		for _, stmt := range fd.file.Shapes {
			switch s := stmt.(type) {
			case *idl.ShapeStmt:
				id, err := ParseShapeID(s.ID)
				if err != nil {
					continue
				}
				for _, tr := range s.Traits {
					if a.resolveRef(fd, tr.Name) == TraitTrait {
						a.traitDefs[id] = true
					}
				}
			case *idl.ApplyStmt:
				target := a.resolveRef(fd, s.Target.ID)
				for _, tr := range s.Traits {
					if a.resolveRef(fd, tr.Name) == TraitTrait {
						a.traitDefs[target] = true
					}
				}
			}
		}
	}
}

// applyShapeTraits applies traits whose targets are shapes (not members).
// Member traits are applied during flattening, after inherited members exist.
func (a *assembler) applyShapeTraits() {
	for _, p := range a.pending {
		if p.applied || p.target.IsMember() {
			continue
		}
		target, ok := a.model.Shape(p.target)
		if !ok {
			continue
		}
		a.applyTrait(target, p)
		p.applied = true
	}

	// Applying @trait through an apply statement can create new trait
	// definitions; record them.
	for _, s := range a.model.Shapes() {
		if s.HasTrait(TraitTrait) {
			a.traitDefs[s.ID] = true
		}
	}
}

func (a *assembler) applyTrait(target *Shape, p *pendingTrait) {
	traitID := TraitDocumentation
	if p.name != "" {
		traitID = a.resolveRef(p.fd, p.name)
	}
	if traitID.IsZero() {
		return
	}

	def, ok := a.model.Shape(traitID)
	if !ok {
		a.danger("Model.UnresolvedTrait", p.source, target.ID,
			"Trait `%s` is not found in the model", traitID)
		return
	}
	if !a.traitDefs[traitID] {
		a.danger("Model.NotATrait", p.source, target.ID,
			"Shape `%s` is not a trait, but it was applied as one", traitID)
		return
	}

	if traitID == TraitMixin && p.fd != nil && !p.fd.version.AtLeast(idl.V2_0) {
		a.danger("Model.InvalidMixin", p.source, target.ID,
			"Mixins can only be used in Smithy 2.0 or later. Attempted to apply a @mixin trait to `%s` in a model file using version `%s`.",
			target.ID, p.fd.version)
	}

	value := nodeFromIDL(p.value, func(raw string) ShapeID { return a.resolveSyntactic(p.fd, raw) })
	if p.annotation {
		switch def.Type {
		case TypeStructure, TypeMap:
			value = Node{Kind: NodeObject, Source: p.source}
		case TypeList, TypeSet:
			value = Node{Kind: NodeArray, Source: p.source}
		case TypeDocument:
			value = Node{Kind: NodeNull, Source: p.source}
		default:
			// Scalar traits require a value. Keep the null node so validation
			// can report Model.InvalidTraitValue.
			value = Node{Kind: NodeNull, Source: p.source}
		}
	}

	trait := &Trait{ID: traitID, Value: value, Source: p.source}
	if existing, ok := target.Traits.Get(traitID); ok {
		if existing.Value.Equal(value) {
			return
		}
		if p.fromApply {
			// apply statements intentionally add or replace traits, including
			// traits inherited from mixins.
			existing.Value = value
			existing.Source = p.source
			return
		}
		if existing.Value.Kind == NodeArray && value.Kind == NodeArray &&
			(def.Type == TypeList || def.Type == TypeSet) {
			existing.Value.Arr = append(existing.Value.Arr, value.Arr...)
			return
		}
		a.danger("Model.Conflict", p.source, target.ID,
			"Trait `%s` conflicts with a previous application on `%s`", traitID, target.ID)
		return
	}
	target.Traits.Set(trait)
}

func (a *assembler) setTrait(target *Shape, trait *Trait) {
	if _, ok := target.Traits.Get(trait.ID); ok {
		return
	}
	target.Traits.Set(trait)
}

func (a *assembler) applyRemainingTraits() {
	for _, p := range a.pending {
		if p.applied {
			continue
		}
		target, ok := a.model.Shape(p.target)
		if !ok {
			if p.fromApply {
				a.danger("Model.UnresolvedShape", p.source, p.target,
					"Cannot apply traits to `%s`: shape is not found in the model", p.target)
			}
			continue
		}
		a.applyTrait(target, p)
		p.applied = true
	}
}

// --- metadata -----------------------------------------------------------------

func (a *assembler) mergeMetadata() {
	metaFD := &fileData{namespace: "", version: idl.V2_0, uses: make(map[string]ShapeID)}
	for _, fd := range a.files {
		for _, m := range fd.file.Metadata {
			value := nodeFromIDL(m.Value, func(raw string) ShapeID {
				return a.resolveSyntactic(metaFD, raw)
			})
			existing, ok := a.model.metadata[m.Key]
			if !ok {
				a.model.metadata[m.Key] = value
				continue
			}
			if existing.Equal(value) {
				continue
			}
			if existing.Kind == NodeArray && value.Kind == NodeArray {
				existing.Arr = append(existing.Arr, value.Arr...)
				a.model.metadata[m.Key] = existing
				continue
			}
			a.danger("Model.MetadataConflict", m.Source, ShapeID{},
				"Metadata key `%s` has conflicting values", m.Key)
		}
	}
}

func (a *assembler) resolveUses() {
	for _, fd := range a.files {
		for _, u := range fd.file.Uses {
			id, err := ParseShapeID(u.ID)
			if err != nil {
				a.danger("Model.UnresolvedShape", u.Source, ShapeID{},
					"Use statement has invalid shape ID: %v", err)
				continue
			}
			a.model.uses = append(a.model.uses, Use{ID: id, Source: u.Source})
			if !a.model.HasShape(id) {
				a.danger("Model.UnresolvedShape", u.Source, id,
					"Use statement targets `%s`, which is not found in the model", id)
			}
		}
	}
	sort.SliceStable(a.model.uses, func(i, j int) bool {
		return a.model.uses[i].ID.String() < a.model.uses[j].ID.String()
	})
}

// --- helpers ------------------------------------------------------------------

func (a *assembler) refsFromNode(fd *fileData, node Node, key string) []ShapeID {
	v, ok := node.Get(key)
	if !ok || v.Kind != NodeArray {
		return nil
	}
	out := make([]ShapeID, 0, len(v.Arr))
	for _, item := range v.Arr {
		if id := a.shapeIDFromNode(fd, item); !id.IsZero() {
			out = append(out, id)
		}
	}
	return out
}

func (a *assembler) shapeIDFromNode(fd *fileData, n Node) ShapeID {
	if n.Kind != NodeString {
		return ShapeID{}
	}
	if id, err := ParseShapeID(n.Str); err == nil && id.IsAbsolute() {
		return id
	}
	if fd != nil {
		return a.resolveRef(fd, n.Str)
	}
	if id, err := ParseShapeID(n.Str); err == nil {
		return id
	}
	return ShapeID{}
}

func appendUniqueRefs(dst []ShapeID, src []ShapeID) []ShapeID {
	seen := make(map[ShapeID]bool, len(dst))
	for _, id := range dst {
		seen[id] = true
	}
	for _, id := range src {
		if id.IsZero() || seen[id] {
			continue
		}
		seen[id] = true
		dst = append(dst, id)
	}
	return dst
}

func hasTraitNamed(a *assembler, fd *fileData, traits []idl.Trait, id ShapeID) bool {
	for _, tr := range traits {
		if a.resolveRef(fd, tr.Name) == id {
			return true
		}
	}
	return false
}
