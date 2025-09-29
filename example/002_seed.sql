insert into authors (name) values
  ('Alice Pennington'),   -- will get 2 books
  ('Marcus Vellum'),      -- will get 1 book
  ('Sofia Duarte'),       -- will get 1 book
  ('Jamal Whitaker'),     -- will get 0 books
  ('Harper Lin');         -- will get 0 books

insert into books (author_id, title)
select id, 'The Clockwork Harbor'
from authors where name = 'Alice Pennington';

insert into books (author_id, title)
select id, 'Shadows in Amber'
from authors where name = 'Alice Pennington';

insert into books (author_id, title)
select id, 'Songs of the Ironwood'
from authors where name = 'Marcus Vellum';

insert into books (author_id, title)
select id, 'The Long Voyage North'
from authors where name = 'Sofia Duarte';

insert into books (author_id, title)
values (null, 'Orphaned Manuscript');

insert into chapters (book_id, title)
select id, 'Chapter 1: Clockwork'
from books where title = 'The Clockwork Harbor';

insert into chapters (book_id, title)
select id, 'Chapter 2: Harbor'
from books where title = 'The Clockwork Harbor';

insert into chapters (book_id, title)
select id, 'Chapter 1: Shadows'
from books where title = 'Shadows in Amber';

insert into chapters (book_id, title)
select id, 'Chapter 2: Amber'
from books where title = 'Shadows in Amber';

insert into chapters (book_id, title)
select id, 'Chapter 1: Iron'
from books where title = 'Songs of the Ironwood';

insert into chapters (book_id, title)
select id, 'Chapter 2: Wood'
from books where title = 'Songs of the Ironwood';
