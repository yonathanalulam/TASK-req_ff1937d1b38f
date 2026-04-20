-- Seed: one user per role
-- Password for all seed users: "password" (development only — never use in production)
-- bcrypt cost 10 hash of "password"
SET @pw = '$2a$10$92IXUNpkjO0rOQ5byMi.Ye4oKoEa3Ro9llC/.og/at2.uheWG/igi';

INSERT IGNORE INTO users (username, email, password_hash, display_name, is_active) VALUES
    ('regular_user',  'regular@example.local',  @pw, 'Regular User',  1),
    ('service_agent', 'agent@example.local',     @pw, 'Service Agent', 1),
    ('moderator',     'moderator@example.local', @pw, 'Moderator',     1),
    ('admin',         'admin@example.local',     @pw, 'Administrator', 1),
    ('data_operator', 'dataop@example.local',    @pw, 'Data Operator', 1);

-- Assign roles
INSERT IGNORE INTO user_roles (user_id, role_id)
SELECT u.id, r.id FROM users u JOIN roles r
    ON (u.username = 'regular_user'  AND r.name = 'regular_user')
UNION ALL
SELECT u.id, r.id FROM users u JOIN roles r
    ON (u.username = 'service_agent' AND r.name = 'service_agent')
UNION ALL
SELECT u.id, r.id FROM users u JOIN roles r
    ON (u.username = 'moderator'     AND r.name = 'moderator')
UNION ALL
SELECT u.id, r.id FROM users u JOIN roles r
    ON (u.username = 'admin'         AND r.name = 'administrator')
UNION ALL
SELECT u.id, r.id FROM users u JOIN roles r
    ON (u.username = 'data_operator' AND r.name = 'data_operator');

-- Default preferences
INSERT IGNORE INTO user_preferences (user_id, notify_in_app)
SELECT id, 1 FROM users;
