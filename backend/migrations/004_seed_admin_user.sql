-- ============================================================================
-- Chaos-Sec: Seed Admin User
-- Migration: 004_seed_admin_user.sql
-- Description: Inserts the default admin user for first-time setup
-- ============================================================================

-- Insert the default admin user.
-- Email:    admin@chaos-sec.local
-- Password: chaos-sec-admin   (bcrypt-hashed, cost 10)
-- Organization: 'Default Organization' (inserted in 001_initial_schema.sql)
-- Role: 'admin' (inserted in 001_initial_schema.sql)
--
-- NOTE: To regenerate the bcrypt hash, run:
--   go run -e -c 'package main; import ("fmt"; "golang.org/x/crypto/bcrypt"); func main() { h, _ := bcrypt.GenerateFromPassword([]byte("chaos-sec-admin"), bcrypt.DefaultCost); fmt.Println(string(h)) }'
INSERT INTO users (email, password_hash, name, organization_id, role_id, is_active)
SELECT
    'admin@chaos-sec.local',
    '$2a$10$9B6BS8ZTa.zEi3qOKzCSH.GC3XkSsGHEaKREb5SxN18jDl6rl7AFm',
    'Chaos-Sec Admin',
    o.id,
    r.id,
    true
FROM organizations o, roles r
WHERE o.slug = 'default' AND r.name = 'admin';

-- Verify the admin user was created.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM users WHERE email = 'admin@chaos-sec.local') THEN
        RAISE WARNING 'Admin user was not created — organization or role seed data may be missing';
    END IF;
END
$$;
