-- Seed: sensitive term dictionary

INSERT IGNORE INTO sensitive_terms (term, class) VALUES
    ('spam',       'prohibited'),
    ('scam',       'prohibited'),
    ('fraud',      'prohibited'),
    ('fake',       'borderline'),
    ('unreliable', 'borderline'),
    ('terrible',   'borderline');
