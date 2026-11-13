package model

import "sort"

func resourceHasProperties(r *ResourceProps) bool {
	if r == nil {
		return false
	}
	return len(r.Identifiers) > 0 || len(r.Properties) > 0 ||
		!r.Create.IsZero() || !r.Read.IsZero() || !r.Update.IsZero() ||
		!r.Delete.IsZero() || !r.List.IsZero() || !r.Put.IsZero() ||
		len(r.Operations) > 0 || len(r.CollectionOperations) > 0 || len(r.Resources) > 0
}

// --- flattening ---------------------------------------------------------------

func (a *assembler) flattenAll() {
	for _, s := range a.model.Shapes() {
		if s.IsMember() {
			continue
		}
		a.flattenShape(s)
	}
}

func (a *assembler) flattenShape(s *Shape) {
	if s.IsMember() {
		return
	}
	switch a.flattenState[s.ID] {
	case flattenDone:
		return
	case flattenVisiting:
		return
	}
	a.flattenState[s.ID] = flattenVisiting

	// Flatten mixins first so their members and traits are complete.
	var validMixins []*Shape
	for _, id := range s.Mixins {
		mixin, ok := a.model.Shape(id)
		if !ok || mixin.Type != s.Type || !mixin.IsMixin() {
			continue
		}
		a.flattenShape(mixin)
		validMixins = append(validMixins, mixin)
	}

	a.resolveElided(s, validMixins)

	// Apply traits that came from shape/member definitions before merging
	// mixins. Apply statements (p.fromApply) are deferred so they can add or
	// replace traits on inherited members.
	a.applyPendingMemberTraits(s, true)

	// Snapshot local traits and member traits so inherited values can be
	// overridden by definitions in the concrete shape.
	localTraits := s.Traits.Clone()
	localMemberTraits := make(map[string]TraitMap, len(s.Members))
	for _, m := range s.Members {
		localMemberTraits[m.ID.Member] = m.Traits.Clone()
	}

	var inheritedMemberOrder []string
	for _, mixin := range validMixins {
		a.mergeMixinTraits(s, mixin, localTraits)
		a.mergeMixinMembers(s, mixin, localMemberTraits, &inheritedMemberOrder)
	}
	a.mergeOperationMixins(s, validMixins)
	a.mergeServiceMixins(s, validMixins)
	a.mergeResourceMixins(s, validMixins)

	a.applyPendingMemberTraits(s, false)
	a.reorderInheritedMembers(s, inheritedMemberOrder)
	a.flattenState[s.ID] = flattenDone
}

// resolveElided sets the targets of elided members from mixins or from a
// resource binding.
func (a *assembler) resolveElided(s *Shape, mixins []*Shape) {
	for _, member := range s.Members {
		if !member.Elided || !member.Target.IsZero() {
			continue
		}
		found := false
		// Resource-bound targets are resolved before mixin targets, matching
		// the reference implementation's modifier order. A discrepancy between
		// the resource binding and a mixin member is reported as a conflict
		// during mixin flattening.
		if !s.ForResource.IsZero() {
			resource, ok := a.model.Shape(s.ForResource)
			if !ok || resource.Type != TypeResource {
				continue
			}
			if resource.Resource != nil {
				if id, ok := resource.Resource.Identifiers[member.ID.Member]; ok {
					member.Target = id
					found = true
				}
				if id, ok := resource.Resource.Properties[member.ID.Member]; ok {
					member.Target = id
					found = true
				}
			}
		}
		if found {
			continue
		}
		for _, mixin := range mixins {
			if mixinMember, ok := mixin.GetMember(member.ID.Member); ok {
				member.Target = mixinMember.Target
				found = true
				break
			}
		}
		if found {
			continue
		}
		a.danger("Model.InvalidMember", member.Source, member.ID,
			"Member target was elided, but no bound resource or mixin contained a matching identifier or member name.")
	}
}

func (a *assembler) mergeMixinTraits(s *Shape, mixin *Shape, localTraits TraitMap) {
	localOnly := make(map[ShapeID]bool)
	for _, id := range mixin.MixinLocalTraits() {
		localOnly[id] = true
	}
	for _, id := range mixin.Traits.SortedIDs() {
		if id == TraitMixin || localOnly[id] || localTraits.Has(id) {
			continue
		}
		s.Traits[id] = mixin.Traits[id]
	}
}

func (a *assembler) mergeMixinMembers(s *Shape, mixin *Shape, localMemberTraits map[string]TraitMap, inheritedOrder *[]string) {
	for _, mixinMember := range mixin.Members {
		name := mixinMember.ID.Member
		if existing, ok := s.GetMember(name); ok {
			if existing.Target.IsZero() {
				existing.Target = mixinMember.Target
			} else if !existing.Target.IsZero() && mixinMember.Target != (ShapeID{}) && existing.Target != mixinMember.Target {
				conflicting := mixinMember.ID
				if _, locallyDefined := localMemberTraits[name]; !locallyDefined {
					conflicting = existing.ID
				}
				a.danger("Model.InvalidMixin", existing.Source, existing.ID,
					"Member conflicts with an inherited mixin member: `%s`", conflicting)
				continue
			}
			local := localMemberTraits[name]
			for _, id := range mixinMember.Traits.SortedIDs() {
				if local.Has(id) {
					continue
				}
				existing.Traits[id] = mixinMember.Traits[id]
			}
			continue
		}

		inherited := NewShape(mixinMember.ID.WithNamespace(s.ID.Namespace), TypeMember)
		inherited.ID = s.ID.WithMember(name)
		inherited.Container = s.ID
		inherited.Target = mixinMember.Target
		inherited.Source = mixinMember.Source
		inherited.Doc = mixinMember.Doc
		inherited.version = s.version
		inherited.prelude = s.prelude
		for id, tr := range mixinMember.Traits {
			inherited.Traits[id] = tr
		}
		s.AddMember(inherited)
		a.model.addShape(inherited)
		*inheritedOrder = append(*inheritedOrder, name)
	}
}

func (a *assembler) mergeOperationMixins(s *Shape, mixins []*Shape) {
	if s.Type != TypeOperation {
		return
	}
	localErrors := append([]ShapeID(nil), s.Errors...)
	var inherited []ShapeID
	for _, mixin := range mixins {
		if mixin.Type != TypeOperation || mixin.Service != nil || mixin.Resource != nil {
			continue
		}
		inherited = appendUniqueRefs(inherited, mixin.Errors)
	}
	if len(inherited) > 0 {
		s.Errors = appendUniqueRefs(inherited, localErrors)
	}
}

func (a *assembler) mergeServiceMixins(s *Shape, mixins []*Shape) {
	if s.Type != TypeService || s.Service == nil {
		return
	}
	local := s.Service
	inherited := &ServiceProps{}
	for _, mixin := range mixins {
		if mixin.Type != TypeService || mixin.Service == nil {
			continue
		}
		if inherited.Version == "" {
			inherited.Version = mixin.Service.Version
		}
		inherited.Operations = appendUniqueRefs(inherited.Operations, mixin.Service.Operations)
		inherited.Resources = appendUniqueRefs(inherited.Resources, mixin.Service.Resources)
		inherited.Errors = appendUniqueRefs(inherited.Errors, mixin.Service.Errors)
		if inherited.Rename == nil {
			inherited.Rename = make(map[ShapeID]string)
		}
		keys := make([]ShapeID, 0, len(mixin.Service.Rename))
		for id := range mixin.Service.Rename {
			keys = append(keys, id)
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
		for _, id := range keys {
			inherited.Rename[id] = mixin.Service.Rename[id]
		}
	}
	if local.Version == "" {
		local.Version = inherited.Version
	}
	local.Operations = appendUniqueRefs(inherited.Operations, local.Operations)
	local.Resources = appendUniqueRefs(inherited.Resources, local.Resources)
	local.Errors = appendUniqueRefs(inherited.Errors, local.Errors)
	if len(inherited.Rename) > 0 {
		merged := make(map[ShapeID]string, len(inherited.Rename)+len(local.Rename))
		for id, name := range inherited.Rename {
			merged[id] = name
		}
		for id, name := range local.Rename {
			merged[id] = name
		}
		local.Rename = merged
	}
}

func (a *assembler) mergeResourceMixins(s *Shape, mixins []*Shape) {
	if s.Type != TypeResource || s.Resource == nil {
		return
	}
	local := s.Resource
	inherited := &ResourceProps{
		Identifiers: make(map[string]ShapeID),
		Properties:  make(map[string]ShapeID),
	}
	for _, mixin := range mixins {
		if mixin.Type != TypeResource || mixin.Resource == nil || resourceHasProperties(mixin.Resource) {
			continue
		}
		for name, id := range mixin.Resource.Identifiers {
			inherited.Identifiers[name] = id
		}
		for name, id := range mixin.Resource.Properties {
			inherited.Properties[name] = id
		}
		if !mixin.Resource.Create.IsZero() && inherited.Create.IsZero() {
			inherited.Create = mixin.Resource.Create
		}
		if !mixin.Resource.Read.IsZero() && inherited.Read.IsZero() {
			inherited.Read = mixin.Resource.Read
		}
		if !mixin.Resource.Update.IsZero() && inherited.Update.IsZero() {
			inherited.Update = mixin.Resource.Update
		}
		if !mixin.Resource.Delete.IsZero() && inherited.Delete.IsZero() {
			inherited.Delete = mixin.Resource.Delete
		}
		if !mixin.Resource.List.IsZero() && inherited.List.IsZero() {
			inherited.List = mixin.Resource.List
		}
		if !mixin.Resource.Put.IsZero() && inherited.Put.IsZero() {
			inherited.Put = mixin.Resource.Put
		}
		inherited.Operations = appendUniqueRefs(inherited.Operations, mixin.Resource.Operations)
		inherited.CollectionOperations = appendUniqueRefs(inherited.CollectionOperations, mixin.Resource.CollectionOperations)
		inherited.Resources = appendUniqueRefs(inherited.Resources, mixin.Resource.Resources)
	}

	for name, id := range inherited.Identifiers {
		if _, ok := local.Identifiers[name]; !ok {
			if local.Identifiers == nil {
				local.Identifiers = make(map[string]ShapeID)
			}
			local.Identifiers[name] = id
		}
	}
	for name, id := range inherited.Properties {
		if _, ok := local.Properties[name]; !ok {
			if local.Properties == nil {
				local.Properties = make(map[string]ShapeID)
			}
			local.Properties[name] = id
		}
	}
	if local.Create.IsZero() {
		local.Create = inherited.Create
	}
	if local.Read.IsZero() {
		local.Read = inherited.Read
	}
	if local.Update.IsZero() {
		local.Update = inherited.Update
	}
	if local.Delete.IsZero() {
		local.Delete = inherited.Delete
	}
	if local.List.IsZero() {
		local.List = inherited.List
	}
	if local.Put.IsZero() {
		local.Put = inherited.Put
	}
	local.Operations = appendUniqueRefs(inherited.Operations, local.Operations)
	local.CollectionOperations = appendUniqueRefs(inherited.CollectionOperations, local.CollectionOperations)
	local.Resources = appendUniqueRefs(inherited.Resources, local.Resources)
}

func (a *assembler) applyPendingMemberTraits(s *Shape, definitionsOnly bool) {
	for _, p := range a.pending {
		if p.applied || !p.target.IsMember() {
			continue
		}
		if definitionsOnly && p.fromApply {
			continue
		}
		if p.target.WithoutMember() != s.ID {
			continue
		}
		target, ok := a.model.Shape(p.target)
		if !ok {
			continue
		}
		a.applyTrait(target, p)
		p.applied = true
	}
}

func (a *assembler) reorderInheritedMembers(s *Shape, inheritedOrder []string) {
	if len(inheritedOrder) == 0 {
		return
	}
	seen := make(map[string]bool, len(s.Members))
	ordered := make([]*Shape, 0, len(s.Members))
	for _, name := range inheritedOrder {
		if m, ok := s.GetMember(name); ok {
			ordered = append(ordered, m)
			seen[name] = true
		}
	}
	for _, m := range s.Members {
		if seen[m.ID.Member] {
			continue
		}
		seen[m.ID.Member] = true
		ordered = append(ordered, m)
	}
	s.Members = ordered
}
