Local Service Commerce & Content Operations Portal

1. questions: Roles are defined but exact permission boundaries between Service Agents, Moderators, Administrators, and Data Operators are unclear
assumptions: Service Agents manage ticket lifecycle, Moderators handle content review and moderation, Administrators have full system access, Data Operators manage ingestion and data pipeline. Users may have multiple roles with RBAC enforced per API request.

2. questions: Offline-first behavior is mentioned but scope of offline functionality and synchronization rules are not defined
assumptions: Users can browse cached data and create service requests offline. Data is stored locally and synced when online. Conflict resolution follows last-write-wins.

3. questions: Service offering structure and lifecycle rules are not fully defined
assumptions: Service offering includes id, name, category, base_price, duration, and active_status. Services can only be enabled or disabled with no versioning or advanced state transitions.

4. questions: Ticket lifecycle transition ownership and control rules are not clearly defined
assumptions: Service Agents control Accepted → Dispatched → In Service → Completed. System/Admin can move tickets to Closed. Transitions are strictly forward-only and cancellation is allowed only before Dispatched.

5. questions: SLA definition and enforcement behavior are not specified
assumptions: SLA is defined per service category using response_time and completion_time stored in database. SLA breaches trigger notifications only, with no automatic escalation or state changes.

6. questions: Shipping rules and ETA calculation logic are not fully defined
assumptions: Shipping costs are calculated using region, weight, and quantity-based rules stored in database. ETA is computed using fixed rule-based offsets within a single system timezone.

7. questions: Review system rules for creation, editing, and rating interpretation are not defined
assumptions: Reviews are allowed only after ticket completion. Ratings ≥4 are considered positive. Users can edit reviews but cannot delete them. Moderators can remove reviews when necessary.

8. questions: Content moderation workflow and violation handling rules are not fully defined
assumptions: Sensitive term dictionary is stored in database. Exact matches are auto-blocked. Borderline content is sent to moderation queue. Repeated violations trigger time-based account restrictions.

9. questions: Notification system delivery channels and failure handling are not defined
assumptions: Notifications are in-app only. Templates are stored in database. Failed notifications are stored in an outbox table for retry tracking and auditing.

10. questions: Data ingestion, storage, and lifecycle rules for lakehouse processing are not fully defined
assumptions: Ingestion runs via scheduled or manual jobs using DB tables and filesystem inputs. Data is processed into Bronze, Silver, and Gold layers stored locally with metadata in MySQL.