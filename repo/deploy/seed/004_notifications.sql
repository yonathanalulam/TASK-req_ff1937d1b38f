-- Seed: notification templates

INSERT IGNORE INTO notification_templates (code, title_template, body_template) VALUES
    ('ticket.status_changed',   'Ticket #{{.TicketID}} Updated',         'Your service request status changed to {{.Status}}.'),
    ('ticket.sla_breach',       'SLA Breach — Ticket #{{.TicketID}}',    'Your ticket has exceeded its SLA deadline. Our team has been notified.'),
    ('ticket.upcoming_start',   'Service Starts Soon — Ticket #{{.TicketID}}', 'Your scheduled service begins in approximately 30 minutes.'),
    ('account.lockout',         'Account Temporarily Locked',            'Your account has been locked for 15 minutes due to multiple failed login attempts.'),
    ('account.posting_freeze',  'Posting Privileges Suspended',          'Your posting privileges have been suspended until {{.FreezeUntil}} due to content policy violations.'),
    ('moderation.approved',     'Your Content Was Approved',             'Content you submitted has been reviewed and approved by a moderator.'),
    ('moderation.rejected',     'Your Content Was Removed',              'Content you submitted has been removed for violating community guidelines.'),
    ('export.ready',            'Your Data Export Is Ready',             'Your requested data export is ready for download. It will expire in 48 hours.'),
    ('review.reminder',         'Review Your Recent Service',            'You recently completed a service request. Share your experience by leaving a review.');
