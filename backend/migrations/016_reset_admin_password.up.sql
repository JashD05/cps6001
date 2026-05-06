-- ============================================================================
-- Chaos-Sec: Reset Default Admin Password
-- Migration: 016_reset_admin_password.up.sql
-- Description: Updates the seeded admin user to use the documented default password
-- ============================================================================

UPDATE users
SET password_hash = '$2a$10$m2.lZ4BkDSPquIZSvdhnJOjJ2Rw6LCZOaDodirQXXk.65vpM45VNi'
WHERE email = 'admin@chaos-sec.local';
