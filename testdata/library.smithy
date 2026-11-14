$version: "2.0"

namespace example.library

/// A small library service used to demonstrate Go code generation.
service Library {
    version: "1.0.0"
    operations: [GetBook, Search, ListBooks]
}

operation GetBook {
    input: GetBookInput
    output: GetBookOutput
    errors: [BookNotFound]
}

operation Search {
    input: SearchInput
    output: SearchOutput
    errors: [BookNotFound]
}

operation ListBooks {
    input: ListBooksInput
    output: ListBooksOutput
}

@error("client")
structure BookNotFound {
    @required
    message: String
    @required
    isbn: ISBN
}

@pattern("^(97[89])?[0-9]{10}$")
string ISBN

@length(min: 1, max: 200)
string Title

@range(min: 0, max: 100)
integer Rating

/// The kind of publication.
enum Genre {
    @enumValue("fiction")
    FICTION

    @enumValue("non-fiction")
    NON_FICTION

    @enumValue("reference")
    REFERENCE
}

/// A coarse reading level.
intEnum ReadingLevel {
    @enumValue(1)
    CHILD

    @enumValue(2)
    TEEN

    @enumValue(3)
    ADULT
}

structure Author {
    @required
    name: String
    bio: String
}

structure Book {
    @required
    isbn: ISBN

    @required
    title: Title

    @required
    genre: Genre

    @required
    author: Author

    rating: Rating
    readingLevel: ReadingLevel
    tags: Tags
}

list Tags {
    member: String
}

structure Magazine {
    @required
    title: String

    @required
    issue: Integer
}

structure ExternalReference {
    @required
    url: String
}

/// A search result is exactly one of the supported publication kinds.
union SearchResult {
    book: Book
    magazine: Magazine
    external: ExternalReference
}

list SearchResults {
    member: SearchResult
}

map Shelf {
    key: String
    value: Book
}

structure GetBookInput {
    @required
    isbn: ISBN
}

structure GetBookOutput {
    @required
    book: Book
    related: SearchResults
}

structure SearchInput {
    @required
    query: String
    genre: Genre
    minRating: Rating
}

structure SearchOutput {
    @required
    results: SearchResults

    @required
    total: Integer
}

list GenreList {
    member: Genre
}

structure ListBooksInput {
    @length(min: 1, max: 20)
    genres: GenreList
}

list BookList {
    member: Book
}

structure ListBooksOutput {
    @required
    books: BookList
    shelves: Shelf
}
