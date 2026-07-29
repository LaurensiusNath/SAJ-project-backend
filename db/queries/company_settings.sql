-- name: GetCompanySettings :one
SELECT * FROM company_settings WHERE id = 1;

-- name: UpdateCompanySettings :one
UPDATE company_settings
SET company_name = $1,
    npwp = $2,
    is_pkp = $3,
    default_tax_percentage = $4,
    default_pph23_rate = $5,
    updated_at = now()
WHERE id = 1
RETURNING *;
