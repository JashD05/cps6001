-- ============================================================================
-- Chaos-Sec: Reset Default Admin Password (Rollback)
-- Migration: 016_reset_admin_password.down.sql
-- Description: Restores the previous seeded admin password
-- ============================================================================

UPDATE users
SET password_hash = '$2a$10$9B6BS8ZTa.zEi3qOKzCSH.GC3XkSsGHEaKREb5SxN18jDl6rl7AFm'
WHERE email = 'admin@chaos-sec.local';
