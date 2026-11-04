package queue

const (
	ClaimJobSQL = `UPDATE sys_jobs 
SET status = 'RUNNING', locked_by = $1, locked_at = now()
WHERE id = (
    SELECT id FROM sys_jobs 
    WHERE status = 'PENDING' AND run_at <= now() 
    ORDER BY run_at ASC 
    FOR UPDATE SKIP LOCKED 
    LIMIT 1
) RETURNING id, tenant_id, workflow_id, task_type, payload, status, run_at, attempts, max_attempts, last_error, locked_by, locked_at, created_at;`

	CompleteJobSQL = `UPDATE sys_jobs 
SET status = 'COMPLETED', locked_by = NULL, locked_at = NULL 
WHERE id = $1;`

	FailJobSQL = `UPDATE sys_jobs 
SET status = 'FAILED', attempts = attempts + 1, last_error = $2, locked_by = NULL, locked_at = NULL 
WHERE id = $1;`

	RetryJobSQL = `UPDATE sys_jobs 
SET status = 'PENDING', attempts = attempts + 1, run_at = $2, last_error = $3, locked_by = NULL, locked_at = NULL 
WHERE id = $1;`

	ReapStrandedJobsSQL = `UPDATE sys_jobs 
SET status = CASE WHEN attempts + 1 >= max_attempts THEN 'FAILED' ELSE 'PENDING' END,
    attempts = attempts + 1,
    last_error = 'worker crash or timeout detected by reaper',
    locked_by = NULL,
    locked_at = NULL,
    run_at = now() + INTERVAL '10 seconds'
WHERE status = 'RUNNING' 
  AND locked_at < now() - ($1 * INTERVAL '1 second');`

	EnqueueJobSQL = `INSERT INTO sys_jobs (tenant_id, workflow_id, task_type, payload, run_at, max_attempts)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, tenant_id, workflow_id, task_type, payload, status, run_at, attempts, max_attempts, last_error, locked_by, locked_at, created_at;`
)
