$version: "2.1"

namespace example.docs

/// A documentation showcase used by the OpenAPI and docs generator tests.
@title("Docs Showcase")
@tags(["library", "beta"])
@externalDocumentation(
    "Homepage": "https://example.com/"
    "API Reference": "https://example.com/api"
)
@unstableFeatures(
    "preview": {
        message: "Preview features are subject to change."
    }
)
service Docs {
    version: "2.1.0"
    operations: [GetBook, CreateBook, PreviewBook]
}

/// Gets a book by id.
@http(method: "GET", uri: "/books/{id}")
@tags(["library"])
@examples([
    {
        title: "Read a book"
        documentation: "Fetches one book by id."
        input: {
            id: "bk-1"
        }
        output: {
            book: {
                id: "bk-1"
                title: "The Book"
                token: "secret"
            }
        }
    }
    {
        title: "Missing book"
        input: {
            id: "bk-404"
        }
        error: {
            shapeId: NotFound
            content: {
                message: "book not found"
            }
        }
    }
])
operation GetBook {
    input: GetBookInput
    output: GetBookOutput
    errors: [NotFound]
}

@http(method: "POST", uri: "/books")
@tags(["library"])
@examples([
    {
        title: "Create a book"
        documentation: "Creates a new book."
        input: {
            book: {
                id: "bk-2"
                title: "Another Book"
            }
        }
        output: {
            book: {
                id: "bk-2"
                title: "Another Book"
            }
        }
    }
])
operation CreateBook {
    input: CreateBookInput
    output: GetBookOutput
}

@title("Preview a book")
@http(method: "POST", uri: "/books/{id}/preview")
@deprecated(message: "Use GetBook.", since: "2.0.0")
@unstable(featureId: "preview")
operation PreviewBook {
    input: GetBookInput
    output: Book
}

structure GetBookInput {
    @required
    @httpLabel
    id: BookId
}

structure CreateBookInput {
    @required
    book: Book
}

/// A book.
@title("Book")
structure Book {
    @required
    id: BookId

    @required
    title: Title

    @deprecated(message: "Do not expose secrets.")
    @internal
    secret: String

    @since("2.1.0")
    @recommended
    @sensitive
    token: SensitiveString

    @unstable(featureId: "preview")
    preview: String

    keywords: [String]

    genre: Genre
}

/// The kind of publication.
enum Genre {
    @documentation("Fiction and literature.")
    @enumValue("fiction")
    FICTION

    @since("2.0.0")
    @enumValue("reference")
    REFERENCE

    @deprecated(message: "Use FICTION or REFERENCE.")
    @enumValue("legacy")
    LEGACY
}

structure GetBookOutput {
    @required
    book: Book
}

@error("client")
@httpError(404)
structure NotFound {
    @required
    message: String
}

/// Identifies a book.
@pattern("^[a-z0-9-]+$")
string BookId

@length(min: 1, max: 100)
string Title

@title("Sensitive string")
@sensitive
string SensitiveString
