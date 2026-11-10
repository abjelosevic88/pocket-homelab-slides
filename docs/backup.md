# Backup

Everything lives in `DATA_DIR`: the SQLite file `pocket-slides.db` (WAL mode; every picture is inside it). Take a consistent copy with `pocket-slides backup` (writes `DATA_DIR/backups/pocket-slides-<time>.db` with `VACUUM INTO`) or Settings → Backup & data → Download database (admin). Back up that copy, not the live file. `DATA_DIR/core.token*` is the Pocket Core registration.

Restore: stop the app, replace `pocket-slides.db` (remove the `-wal`/`-shm` files), start.

Every person can also export everything they made — decks with their slides and pictures, their themes and layouts, talks and rehearsals with the questions, poll answers and feedback — as one JSON file (Settings → Backup & data), and bring it back on any box; importing the same file twice adds nothing twice.
