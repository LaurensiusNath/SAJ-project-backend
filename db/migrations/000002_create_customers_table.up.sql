CREATE TABLE customers (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    customer_type VARCHAR(20) NOT NULL CHECK (customer_type IN ('badan_usaha', 'perorangan')),
    phone VARCHAR(30),
    email VARCHAR(255),
    address TEXT,
    company_name VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_customers_name ON customers(name) WHERE deleted_at IS NULL;
