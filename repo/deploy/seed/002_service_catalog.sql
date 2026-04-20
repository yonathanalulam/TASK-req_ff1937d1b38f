-- Seed: service categories and sample offerings

INSERT IGNORE INTO service_categories (name, slug, description, response_time_minutes, completion_time_minutes) VALUES
    ('Plumbing',          'plumbing',          'Pipe, drain, and fixture services',      60,  240),
    ('Electrical',        'electrical',        'Wiring, outlets, and panel work',        90,  360),
    ('HVAC',              'hvac',              'Heating, ventilation, and air conditioning', 120, 480),
    ('Cleaning',          'cleaning',          'Residential and commercial cleaning',    30,  180),
    ('Landscaping',       'landscaping',       'Lawn care, pruning, and gardening',      60,  300);

-- Sample offerings (linked to service_agent user)
INSERT IGNORE INTO service_offerings (agent_id, category_id, name, description, base_price, duration_minutes, active_status)
SELECT
    u.id,
    c.id,
    'Basic Plumbing Inspection',
    'Full inspection of household plumbing system including pipes, fixtures, and drainage.',
    89.99,
    90,
    1
FROM users u JOIN service_categories c ON c.slug = 'plumbing'
WHERE u.username = 'service_agent'
LIMIT 1;

INSERT IGNORE INTO service_offerings (agent_id, category_id, name, description, base_price, duration_minutes, active_status)
SELECT
    u.id,
    c.id,
    'Electrical Safety Audit',
    'Comprehensive electrical panel and wiring safety inspection.',
    129.99,
    120,
    1
FROM users u JOIN service_categories c ON c.slug = 'electrical'
WHERE u.username = 'service_agent'
LIMIT 1;

INSERT IGNORE INTO service_offerings (agent_id, category_id, name, description, base_price, duration_minutes, active_status)
SELECT
    u.id,
    c.id,
    'Deep Home Cleaning',
    'Full home deep clean including kitchen, bathrooms, and all rooms.',
    149.99,
    240,
    1
FROM users u JOIN service_categories c ON c.slug = 'cleaning'
WHERE u.username = 'service_agent'
LIMIT 1;
