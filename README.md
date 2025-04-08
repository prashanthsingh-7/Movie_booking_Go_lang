# Student Registration System

A simple Go web application for managing student records with MySQL database integration.

## Features

- List all students
- Add new students
- Edit existing students
- Delete students
- Persistent storage with MySQL

## Prerequisites

- Go 1.16 or higher
- MySQL Server 5.7 or higher

## Setup Instructions

### 1. Install Go MySQL driver

```bash
go get -u github.com/go-sql-driver/mysql
```

### 2. Set up MySQL Database

1. Log in to MySQL:

```bash
mysql -u root -p
```

2. Create a database:

```sql
CREATE DATABASE student_db;
USE student_db;
```

3. Note: The application will automatically create the required tables on startup.

### 3. Configure Database Connection

In `main.go`, modify the database connection string if needed:

```go
db, err = sql.Open("mysql", "root:password@tcp(127.0.0.1:3306)/student_db")
```

Replace `root` with your MySQL username and `password` with your MySQL password.

### 4. Run the Application

```bash
go run main.go
```

The application will be available at http://localhost:8080

## Application Structure

- `main.go` - Main application file with all the routes and database operations
- `templates/` - Directory containing HTML templates
  - `index.gohtml` - Template for displaying all students
  - `create.gohtml` - Template for adding a new student
  - `edit.gohtml` - Template for editing an existing student

## API Endpoints

- `GET /` - Redirects to /students
- `GET /students` - Shows all students
- `GET /student/create` - Shows the form to add a new student
- `POST /student/insert` - Adds a new student to the database
- `GET /student/edit/{id}` - Shows the form to edit an existing student
- `POST /student/update` - Updates an existing student
- `GET /student/delete/{id}` - Deletes a student 