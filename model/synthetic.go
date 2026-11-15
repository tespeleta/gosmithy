package model

import (
	"strings"

	"github.com/tespeleta/gosmithy/idl"
)

// syntheticPrefix is the reserved name prefix for assembler-generated shapes.
const syntheticPrefix = "_Synthetic"

// declareInlineCollection creates (or reuses) the synthetic list or map shape
// for an IDL 2.1 inline collection and returns its shape ID.
func (a *assembler) declareInlineCollection(namespace string, target idl.Target, fd *fileData) ShapeID {
	switch {
	case target.InlineList != nil:
		element := a.resolveInlineTarget(namespace, target.InlineList.Element, fd)
		if element.IsZero() {
			return ShapeID{}
		}
		id := NewShapeID(namespace, syntheticPrefix+"ListOf"+syntheticToken(namespace, element))
		a.createSyntheticCollection(id, TypeList, element, ShapeID{},
			"list:"+element.String(), target.Source, fd)
		return id
	case target.InlineMap != nil:
		key := a.resolveInlineTarget(namespace, target.InlineMap.Key, fd)
		value := a.resolveInlineTarget(namespace, target.InlineMap.Value, fd)
		if key.IsZero() || value.IsZero() {
			return ShapeID{}
		}
		name := syntheticPrefix + "MapOf" + syntheticToken(namespace, key) +
			"_To_" + syntheticToken(namespace, value)
		id := NewShapeID(namespace, name)
		a.createSyntheticCollection(id, TypeMap, key, value,
			"map:"+key.String()+","+value.String(), target.Source, fd)
		return id
	default:
		return a.resolveRef(fd, target.ID)
	}
}

// resolveInlineTarget resolves one element of an inline collection, recursing
// into nested collections. Nested collections are scoped to the namespace of
// the structure that declares the outermost collection.
func (a *assembler) resolveInlineTarget(namespace string, target idl.Target, fd *fileData) ShapeID {
	if target.IsInline() {
		return a.declareInlineCollection(namespace, target, fd)
	}
	return a.resolveRef(fd, target.ID)
}

// syntheticToken encodes a resolved target shape ID into the token used in a
// synthetic shape name. The encoding is decided only from the target's
// namespace and name, so names are stable regardless of declaration order or
// what else the model contains.
func syntheticToken(containingNamespace string, target ShapeID) string {
	sameNamespace := target.Namespace == containingNamespace
	if sameNamespace && strings.HasPrefix(target.Name, syntheticPrefix) {
		return "_" + target.Name
	}
	if !strings.HasPrefix(target.Name, "_") {
		if target.Namespace == PreludeNamespace {
			return "__" + target.Name
		}
		if sameNamespace {
			return "_" + target.Name
		}
	}
	return "_" + strings.ReplaceAll(target.Namespace, ".", "_") + "_" + target.Name
}

// createSyntheticCollection adds a synthetic list or map shape and its
// member(s), or reuses an identical collection that already exists. Identical
// collections in a namespace are grouped on the generated ID; a different
// collection that maps to the same ID is reported as a conflict.
func (a *assembler) createSyntheticCollection(id ShapeID, typ ShapeType, key, value ShapeID, signature string, loc idl.SourceLocation, fd *fileData) {
	if existing, ok := a.model.Shape(id); ok {
		if a.syntheticSignatures[id] == signature {
			return
		}
		if previous, ok := a.syntheticSignatures[id]; ok {
			a.danger("Model.DuplicateShapeId", loc, id,
				"Synthetic collection `%s` (%s) conflicts with an earlier inline collection (%s) that generated the same name",
				id, signature, previous)
		} else {
			a.danger("Model.DuplicateShapeId", loc, id,
				"Cannot create synthetic collection `%s`: shape `%s` is already defined", id, existing.ID)
		}
		return
	}

	shape := NewShape(id, typ)
	shape.Source = loc
	shape.prelude = fd.prelude
	shape.version = fd.version
	a.model.addShape(shape)
	a.caseIndex[strings.ToLower(id.String())] = id
	a.syntheticSignatures[id] = signature
	shape.Traits.Set(&Trait{
		ID:     TraitSynthetic,
		Value:  Node{Kind: NodeObject, Source: loc},
		Source: loc,
	})

	addMember := func(name string, target ShapeID) {
		member := NewShape(id.WithMember(name), TypeMember)
		member.Container = id
		member.Target = target
		member.Source = loc
		member.prelude = fd.prelude
		member.version = fd.version
		shape.AddMember(member)
		a.model.addShape(member)
	}
	switch typ {
	case TypeList:
		addMember("member", key)
	case TypeMap:
		addMember("key", key)
		addMember("value", value)
	}
}
