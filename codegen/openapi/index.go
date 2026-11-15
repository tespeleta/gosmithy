package openapi

import (
	"fmt"
	"html"
)

func renderHTML(doc *Document) []byte {
	title := html.EscapeString(doc.Info.Title + " API Reference")
	description := html.EscapeString(doc.Info.Description)
	return []byte(fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>%s</title>
  <meta name="description" content="%s">
  <style>html, body { margin: 0; padding: 0; }</style>
</head>
<body>
  <redoc spec-url="./openapi.json"></redoc>
  <noscript>
    <p>This API reference needs JavaScript. The raw OpenAPI document is
    available at <a href="./openapi.json">openapi.json</a>, and the generated
    Markdown guide is in <a href="./README.md">README.md</a>.</p>
  </noscript>
  <script type="module" src="https://cdn.redoc.ly/redoc/v3.x/bundles/redoc.standalone.js"></script>
</body>
</html>
`, title, description))
}
