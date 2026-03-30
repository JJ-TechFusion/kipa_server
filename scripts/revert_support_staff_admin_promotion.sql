-- Identify support staff accounts that currently also have admin privileges.
SELECT
  u.id,
  u.phone_number,
  u.first_name,
  u.last_name,
  u.is_admin,
  ss.is_active,
  ss.can_reply_chats,
  ss.can_view_disputes,
  ss.can_resolve_chats,
  ss.can_message_users,
  ss.created_at AS support_staff_created_at
FROM users u
JOIN support_staff ss ON ss.user_id = u.id
WHERE u.is_admin = TRUE
ORDER BY ss.created_at DESC;

-- After manually confirming which of the rows above are accidental promotions,
-- replace the UUID list below and run this update to revoke unintended admin access.
--
-- UPDATE users
-- SET is_admin = FALSE,
--     updated_at = NOW()
-- WHERE id IN (
--   '00000000-0000-0000-0000-000000000000'
-- );
