-- Seed: shipping regions and fee templates

INSERT IGNORE INTO shipping_regions (name, cutoff_time, timezone) VALUES
    ('Northeast', '17:00:00', 'America/New_York'),
    ('Southeast', '16:00:00', 'America/New_York'),
    ('Midwest',   '16:00:00', 'America/Chicago'),
    ('West',      '15:00:00', 'America/Los_Angeles');

-- Courier templates for Northeast region
INSERT IGNORE INTO shipping_templates (region_id, delivery_method, min_weight_kg, max_weight_kg, min_quantity, max_quantity, fee_amount, currency, lead_time_hours, window_hours)
SELECT r.id, 'courier', 0,    5,    1, 9999, 9.99,  'USD', 24, 4 FROM shipping_regions r WHERE r.name = 'Northeast'
UNION ALL
SELECT r.id, 'courier', 5,    20,   1, 9999, 19.99, 'USD', 48, 4 FROM shipping_regions r WHERE r.name = 'Northeast'
UNION ALL
SELECT r.id, 'courier', 20, 999,   1, 9999, 39.99, 'USD', 72, 4 FROM shipping_regions r WHERE r.name = 'Northeast'
UNION ALL
SELECT r.id, 'pickup',  0,  999,   1, 9999, 0.00,  'USD', 0,  0 FROM shipping_regions r WHERE r.name = 'Northeast';
