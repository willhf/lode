-- designed for sqlite

CREATE TABLE IF NOT EXISTS authors (
  id    INTEGER PRIMARY KEY,
  name  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS books (
  id         INTEGER PRIMARY KEY,
  author_id  INTEGER REFERENCES authors(id) ON DELETE CASCADE,
  title      TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS chapters (
  id       INTEGER PRIMARY KEY,
  book_id  INTEGER REFERENCES books(id) ON DELETE CASCADE,
  title    TEXT NOT NULL
);
