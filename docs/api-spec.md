# API Specification

## Overview

This document describes the REST API for the Eagle Point Service Portal. The API provides endpoints for authentication, user management, service catalog, tickets, reviews, and administrative functions.

## Base URL

```
http://localhost:8080/api/v1
```

## Authentication

The API uses session-based authentication with CSRF protection. All protected endpoints require:
- Valid session cookie
- CSRF token in headers

### Authentication Endpoints

#### POST /auth/login
Authenticates a user and creates a session.

**Request Body:**
```json
{
  "email": "string",
  "password": "string"
}
```

**Response:**
```json
{
  "success": true,
  "user": {
    "id": "uuid",
    "email": "string",
    "name": "string",
    "role": "string"
  }
}
```

#### POST /auth/logout
Terminates the user session.

**Response:**
```json
{
  "success": true
}
```

#### GET /auth/me
Returns current user information.

**Response:**
```json
{
  "id": "uuid",
  "email": "string",
  "name": "string",
  "role": "string",
  "profile": {
    "address": "string",
    "phone": "string"
  }
}
```

## User Profile Management

#### GET /profile
Retrieves user profile information.

#### PUT /profile
Updates user profile information.

**Request Body:**
```json
{
  "name": "string",
  "address": "string",
  "phone": "string"
}
```

## Service Catalog

#### GET /catalog/services
Retrieves list of available services.

**Response:**
```json
{
  "services": [
    {
      "id": "uuid",
      "name": "string",
      "description": "string",
      "category": "string",
      "price": "number"
    }
  ]
}
```

#### GET /catalog/services/{id}
Retrieves specific service details.

## Tickets

#### GET /tickets
Retrieves user's tickets.

**Query Parameters:**
- `status`: filter by status (open, closed, pending)
- `limit`: number of results to return
- `offset`: pagination offset

#### POST /tickets
Creates a new support ticket.

**Request Body:**
```json
{
  "subject": "string",
  "description": "string",
  "priority": "low|medium|high",
  "service_id": "uuid"
}
```

#### GET /tickets/{id}
Retrieves specific ticket details.

#### PUT /tickets/{id}
Updates ticket status or adds comments.

## Reviews

#### GET /reviews
Retrieves service reviews.

#### POST /reviews
Submits a new service review.

**Request Body:**
```json
{
  "service_id": "uuid",
  "rating": "number",
  "comment": "string"
}
```

## Notifications

#### GET /notifications
Retrieves user notifications.

#### PUT /notifications/{id}/read
Marks notification as read.

## Admin Endpoints

#### GET /admin/users
Retrieves list of users (admin only).

#### GET /admin/audit
Retrieves audit logs (admin only).

#### POST /admin/ingest
Bulk data ingestion endpoint (admin only).

## Health Check

#### GET /health
Returns service health status.

**Response:**
```json
{
  "status": "healthy",
  "timestamp": "ISO8601",
  "version": "string"
}
```

## Error Responses

All endpoints return standard error responses:

```json
{
  "error": {
    "code": "string",
    "message": "string",
    "details": "object"
  }
}
```

### Common Error Codes

- `UNAUTHORIZED`: Invalid or missing authentication
- `FORBIDDEN`: Insufficient permissions
- `NOT_FOUND`: Resource not found
- `VALIDATION_ERROR`: Invalid request data
- `RATE_LIMITED`: Too many requests
- `INTERNAL_ERROR`: Server error

## Rate Limiting

The API implements rate limiting:
- General endpoints: 100 requests per minute
- Review/report endpoints: 10 requests per minute

## Security Features

- CSRF protection on all state-changing endpoints
- Session-based authentication with secure cookies
- Field-level encryption for sensitive data
- HMAC verification for internal API calls
- Request logging and audit trails

## Data Models

### User
```json
{
  "id": "uuid",
  "email": "string",
  "name": "string",
  "role": "user|admin|moderator",
  "created_at": "datetime",
  "updated_at": "datetime"
}
```

### Service
```json
{
  "id": "uuid",
  "name": "string",
  "description": "string",
  "category": "string",
  "price": "decimal",
  "created_at": "datetime",
  "updated_at": "datetime"
}
```

### Ticket
```json
{
  "id": "uuid",
  "user_id": "uuid",
  "subject": "string",
  "description": "string",
  "status": "open|closed|pending",
  "priority": "low|medium|high",
  "created_at": "datetime",
  "updated_at": "datetime"
}
```

### Review
```json
{
  "id": "uuid",
  "user_id": "uuid",
  "service_id": "uuid",
  "rating": "number",
  "comment": "string",
  "status": "pending|approved|rejected",
  "created_at": "datetime"
}
```
