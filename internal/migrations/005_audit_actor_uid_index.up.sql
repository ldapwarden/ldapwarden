-- The audit page filters entries by actor uid and offers the distinct actors as
-- a dropdown (GROUP BY actor_uid ORDER BY MAX(created_at) DESC). Both scan the
-- whole table without an index on actor_uid: the existing index covers actor_dn,
-- which is not what either query keys off.
--
-- audit_logs is append-only and never pruned, so that scan cost grows without
-- bound — the dropdown alone would re-scan every row each time the page opens.
-- created_at is part of the index so the actor listing can get its MAX per uid
-- from the index instead of visiting the heap.
--
-- Plain CREATE INDEX rather than CONCURRENTLY: migrations run at startup before
-- the server accepts traffic, so the brief write lock on audit_logs blocks
-- nothing, and a failed CONCURRENTLY build would leave an INVALID index behind.
CREATE INDEX IF NOT EXISTS idx_audit_logs_actor_uid_created_at
    ON audit_logs (actor_uid, created_at DESC);
