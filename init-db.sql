-- MoopicView Database Schema

-- Users table
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255),
    name VARCHAR(255),
    google_id VARCHAR(255),
    role VARCHAR(50) DEFAULT 'user',
    approved BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Account requests table
CREATE TABLE account_requests (
    id SERIAL PRIMARY KEY,
    email VARCHAR(255) UNIQUE NOT NULL,
    name VARCHAR(255),
    message TEXT,
    status VARCHAR(50) DEFAULT 'pending',
    reviewed_by INTEGER REFERENCES users(id),
    reviewed_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Folders table (for hierarchical organization)
CREATE TABLE folders (
    id SERIAL PRIMARY KEY,
    path VARCHAR(1024) UNIQUE NOT NULL,
    name VARCHAR(255),
    parent_path VARCHAR(1024),
    collection_type VARCHAR(50) -- 'digital' or 'scanned'
);

-- Photos table
CREATE TABLE photos (
    id SERIAL PRIMARY KEY,
    filepath VARCHAR(1024) UNIQUE NOT NULL,
    filename VARCHAR(255),
    collection VARCHAR(50), -- 'digital' or 'scanned'
    folder_id INTEGER REFERENCES folders(id),
    scan_date DATE,
    photo_date DATE,
    date_precision VARCHAR(10), -- 'exact', 'month', 'year', 'unknown'
    date_source VARCHAR(20), -- 'exif', 'directory', 'manual', 'estimated', 'unknown'
    description TEXT,
    original_date TIMESTAMP,
    width INTEGER,
    height INTEGER,
    imported_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Tags table
CREATE TABLE tags (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) UNIQUE NOT NULL
);

-- Photo tags junction table with position
CREATE TABLE photo_tags (
    photo_id INTEGER REFERENCES photos(id) ON DELETE CASCADE,
    tag_id INTEGER REFERENCES tags(id) ON DELETE CASCADE,
    pos_x FLOAT DEFAULT 50, -- X position as percentage (0-100)
    pos_y FLOAT DEFAULT 50, -- Y position as percentage (0-100)
    PRIMARY KEY (photo_id, tag_id)
);

-- Comments table
CREATE TABLE comments (
    id SERIAL PRIMARY KEY,
    photo_id INTEGER REFERENCES photos(id) ON DELETE CASCADE,
    user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Proposed edits table
CREATE TABLE proposed_edits (
    id SERIAL PRIMARY KEY,
    photo_id INTEGER REFERENCES photos(id) ON DELETE CASCADE,
    user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
    field VARCHAR(50) NOT NULL, -- 'description' or 'date'
    proposed_value TEXT,
    current_value TEXT,
    status VARCHAR(50) DEFAULT 'pending', -- pending/approved/rejected
    reviewed_by INTEGER REFERENCES users(id),
    reviewed_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Password resets table
CREATE TABLE password_resets (
    id SERIAL PRIMARY KEY,
    email VARCHAR(255) NOT NULL,
    token VARCHAR(255) UNIQUE NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    used BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Activity logs table
CREATE TABLE activity_logs (
    id SERIAL PRIMARY KEY,
    user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
    action VARCHAR(255) NOT NULL,
    entity_type VARCHAR(255),
    entity_id INTEGER,
    details TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes for performance
CREATE INDEX idx_photos_filepath ON photos(filepath);
CREATE INDEX idx_photos_folder_id ON photos(folder_id);
CREATE INDEX idx_photos_photo_date ON photos(photo_date);
CREATE INDEX idx_photos_collection ON photos(collection);
CREATE INDEX idx_folders_path ON folders(path);
CREATE INDEX idx_folders_parent_path ON folders(parent_path);
CREATE INDEX idx_comments_photo_id ON comments(photo_id);
CREATE INDEX idx_comments_user_id ON comments(user_id);
CREATE INDEX idx_proposed_edits_photo_id ON proposed_edits(photo_id);
CREATE INDEX idx_proposed_edits_status ON proposed_edits(status);

-- Create the first admin user (password will be set via CLI or setup)
INSERT INTO users (email, name, role, approved) VALUES (
    'admin@moopicview.local',
    'Admin User',
    'admin',
    TRUE
) ON CONFLICT (email) DO NOTHING;
