-- Enables the optional pageinspect extension so the lab can later expose
-- low-level B-tree page inspection (e.g. bt_page_items, bt_metap).
CREATE EXTENSION IF NOT EXISTS pageinspect;
