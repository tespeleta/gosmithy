package model

import "sort"

// ServiceClosure returns the transitive closure of the shapes reachable from a
// service shape, in deterministic lexical order.
//
// The closure includes the service itself, operations, resources (recursively),
// resource identifiers and properties, operation input/output/errors, every
// member shape, and every member target. Trait-only references are not
// followed. Shapes flattened from mixins are reachable through their members;
// mixin shapes themselves are not traversed.
//
// The second return value is false when id does not name a service shape.
func (m *Model) ServiceClosure(id ShapeID) ([]ShapeID, bool) {
	service, ok := m.Shape(id)
	if !ok || service.Type != TypeService {
		return nil, false
	}

	visited := make(map[ShapeID]bool)
	queue := []ShapeID{id}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if visited[current] {
			continue
		}
		shape, ok := m.Shape(current)
		if !ok {
			continue
		}
		visited[current] = true

		// Member shapes are first-class shapes in the closure. Their targets
		// are reachable too.
		for _, member := range shape.Members {
			if !visited[member.ID] {
				queue = append(queue, member.ID)
			}
			if !member.Target.IsZero() && !visited[member.Target] {
				queue = append(queue, member.Target)
			}
		}

		switch shape.Type {
		case TypeOperation:
			for _, target := range []ShapeID{shape.Input, shape.Output} {
				if !target.IsZero() && !visited[target] {
					queue = append(queue, target)
				}
			}
			for _, errID := range shape.Errors {
				if !visited[errID] {
					queue = append(queue, errID)
				}
			}
		case TypeService:
			if shape.Service != nil {
				for _, list := range [][]ShapeID{
					shape.Service.Operations,
					shape.Service.Resources,
					shape.Service.Errors,
				} {
					for _, ref := range list {
						if !visited[ref] {
							queue = append(queue, ref)
						}
					}
				}
			}
		case TypeResource:
			if shape.Resource != nil {
				r := shape.Resource
				for _, list := range [][]ShapeID{
					{r.Create, r.Read, r.Update, r.Delete, r.List, r.Put},
					r.Operations,
					r.CollectionOperations,
					r.Resources,
				} {
					for _, ref := range list {
						if !ref.IsZero() && !visited[ref] {
							queue = append(queue, ref)
						}
					}
				}
				for _, target := range r.Identifiers {
					if !visited[target] {
						queue = append(queue, target)
					}
				}
				for _, target := range r.Properties {
					if !visited[target] {
						queue = append(queue, target)
					}
				}
			}
		}
	}

	out := make([]ShapeID, 0, len(visited))
	for id := range visited {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out, true
}

// ServiceOperationErrors returns the effective error shapes for an operation
// bound to a service: the operation's own errors plus the service's common
// errors, in deterministic order.
func (m *Model) ServiceOperationErrors(serviceID, operationID ShapeID) []ShapeID {
	service, ok := m.Shape(serviceID)
	if !ok || service.Type != TypeService || service.Service == nil {
		return nil
	}
	op, ok := m.Shape(operationID)
	if !ok || op.Type != TypeOperation {
		return nil
	}
	seen := make(map[ShapeID]bool)
	var out []ShapeID
	for _, errID := range append(append([]ShapeID(nil), op.Errors...), service.Service.Errors...) {
		if errID.IsZero() || seen[errID] {
			continue
		}
		seen[errID] = true
		out = append(out, errID)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}
