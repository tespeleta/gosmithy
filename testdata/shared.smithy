$version: "2.0"

// Port of the Java repository's valid/__shared.json fixture. M2 has no JSON AST
// loader, so the shared model is expressed as IDL instead.
namespace foo.example

structure Widget {
    id: String
}
