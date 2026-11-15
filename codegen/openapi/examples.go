package openapi

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/tespeleta/gosmithy/model"
)

var traitExamples = model.MustShapeID("smithy.api#examples")

type operationExample struct {
	Title         string
	Documentation string
	Input         *model.Node
	Output        *model.Node
	Error         *errorExample
}

type errorExample struct {
	Shape   model.ShapeID
	Content *model.Node
}

func (b *builder) operationExamples(op *model.Shape) []operationExample {
	trait, ok := op.GetTrait(traitExamples)
	if !ok || trait.Value.Kind != model.NodeArray {
		return nil
	}
	examples := make([]operationExample, 0, len(trait.Value.Arr))
	for _, entry := range trait.Value.Arr {
		if entry.Kind != model.NodeObject {
			continue
		}
		title, _ := entry.GetString("title")
		if title == "" {
			continue
		}
		example := operationExample{Title: title}
		example.Documentation, _ = entry.GetString("documentation")
		if input, ok := entry.Get("input"); ok && input.Kind == model.NodeObject {
			value := input
			example.Input = &value
		}
		if output, ok := entry.Get("output"); ok && output.Kind == model.NodeObject {
			value := output
			example.Output = &value
		}
		if errorNode, ok := entry.Get("error"); ok && errorNode.Kind == model.NodeObject {
			shapeID, _ := errorNode.GetString("shapeId")
			if id := b.exampleShapeID(shapeID); !id.IsZero() {
				errExample := &errorExample{Shape: id}
				if content, ok := errorNode.Get("content"); ok && content.Kind == model.NodeObject {
					value := content
					errExample.Content = &value
				}
				example.Error = errExample
			}
		}
		examples = append(examples, example)
	}
	return examples
}

func (b *builder) exampleShapeID(value string) model.ShapeID {
	id, err := model.ParseShapeID(value)
	if err != nil {
		return model.ShapeID{}
	}
	if id.IsAbsolute() {
		return id
	}
	return model.NewShapeID(b.service.ID.Namespace, id.Name)
}

func (b *builder) bodyExamples(members []model.ShapeID, payload model.ShapeID, examples []operationExample, pick func(operationExample) *model.Node) map[string]*Example {
	var result map[string]*Example
	for _, example := range examples {
		value := pick(example)
		if value == nil {
			continue
		}
		var exampleValue interface{}
		if !payload.IsZero() {
			member, ok := b.model.Shape(payload)
			if !ok {
				continue
			}
			field, ok := value.Get(member.ID.Member)
			if !ok {
				if jsonName, ok := member.JSONName(); ok && jsonName != "" {
					field, ok = value.Get(jsonName)
				}
			}
			if !ok {
				continue
			}
			exampleValue = nodeToAny(field)
		} else {
			object := make(map[string]interface{})
			for _, id := range members {
				member, ok := b.model.Shape(id)
				if !ok {
					continue
				}
				field, ok := value.Get(member.ID.Member)
				if !ok {
					continue
				}
				name := member.ID.Member
				if jsonName, ok := member.JSONName(); ok && jsonName != "" {
					name = jsonName
				}
				object[name] = nodeToAny(field)
			}
			if len(object) == 0 {
				continue
			}
			exampleValue = object
		}
		if result == nil {
			result = make(map[string]*Example)
		}
		result[uniqueExampleKey(result, example.Title)] = &Example{
			Summary:     example.Title,
			Description: example.Documentation,
			Value:       exampleValue,
		}
	}
	return result
}

func (b *builder) parameterExamples(member *model.Shape, examples []operationExample) map[string]*Example {
	var result map[string]*Example
	for _, example := range examples {
		if example.Input == nil {
			continue
		}
		value, ok := example.Input.Get(member.ID.Member)
		if !ok {
			continue
		}
		if result == nil {
			result = make(map[string]*Example)
		}
		result[uniqueExampleKey(result, example.Title)] = &Example{
			Summary:     example.Title,
			Description: example.Documentation,
			Value:       nodeToAny(value),
		}
	}
	return result
}

func (b *builder) errorExamples(examples []operationExample) map[model.ShapeID]map[string]*Example {
	result := make(map[model.ShapeID]map[string]*Example)
	for _, example := range examples {
		if example.Error == nil {
			continue
		}
		value := map[string]interface{}{}
		if example.Error.Content != nil {
			value = nodeObjectToMap(*example.Error.Content)
		}
		key := example.Error.Shape
		if result[key] == nil {
			result[key] = make(map[string]*Example)
		}
		result[key][uniqueExampleKey(result[key], example.Title)] = &Example{
			Summary:     example.Title,
			Description: example.Documentation,
			Value:       value,
		}
	}
	return result
}

func uniqueExampleKey(existing map[string]*Example, base string) string {
	if _, ok := existing[base]; !ok {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s %d", base, i)
		if _, ok := existing[candidate]; !ok {
			return candidate
		}
	}
}

func nodeToAny(node model.Node) interface{} {
	switch node.Kind {
	case model.NodeNull:
		return nil
	case model.NodeBoolean:
		return node.Bool
	case model.NodeNumber:
		return json.Number(node.Num)
	case model.NodeString:
		return node.Str
	case model.NodeArray:
		values := make([]interface{}, 0, len(node.Arr))
		for _, child := range node.Arr {
			values = append(values, nodeToAny(child))
		}
		return values
	case model.NodeObject:
		return nodeObjectToMap(node)
	default:
		return nil
	}
}

func nodeObjectToMap(node model.Node) map[string]interface{} {
	values := make(map[string]interface{}, len(node.Obj))
	keys := make([]string, 0, len(node.Obj))
	for _, entry := range node.Obj {
		keys = append(keys, entry.Key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value, _ := node.Get(key)
		values[key] = nodeToAny(value)
	}
	return values
}
