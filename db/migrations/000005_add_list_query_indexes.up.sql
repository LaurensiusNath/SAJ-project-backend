-- Mendukung ORDER BY created_at DESC di ListCustomers/ListJobs, yang
-- sebelumnya selalu jatuh ke in-memory sort (top-N heapsort) karena tidak
-- ada index yang urutannya cocok. Lihat hasil EXPLAIN ANALYZE di PR ini.
CREATE INDEX idx_customers_created_at ON customers (created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_jobs_created_at ON jobs (created_at DESC);
