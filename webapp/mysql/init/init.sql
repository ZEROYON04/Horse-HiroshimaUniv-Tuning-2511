USE `hiroshimauniv2511-db`;

DROP TABLE IF EXISTS user_sessions;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS products;
DROP TABLE IF EXISTS `users`;

CREATE TABLE `users` (
  `user_id` INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `password_hash` VARCHAR(255) NOT NULL,
  `user_name` VARCHAR(255) NOT NULL
  );

-- productsテーブルの作成
CREATE TABLE products (
    product_id INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    value INT UNSIGNED NOT NULL,
    weight INT UNSIGNED NOT NULL,
    image VARCHAR(500),
    description TEXT
) ENGINE=InnoDB
DEFAULT CHARSET=utf8mb4
COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE orders (
    order_id INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id INT UNSIGNED NOT NULL,
    product_id INT UNSIGNED NOT NULL,
    shipped_status VARCHAR(50) NOT NULL,
    created_at DATETIME NOT NULL,
    arrived_at DATETIME,
    FOREIGN KEY (user_id) REFERENCES users(user_id) ON DELETE CASCADE,
    FOREIGN KEY (product_id) REFERENCES products(product_id) ON DELETE CASCADE
);

CREATE TABLE `user_sessions` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `session_uuid` VARCHAR(36) NOT NULL,
  `user_id` INT UNSIGNED NOT NULL,
  `expires_at` DATETIME NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `session_uuid` (`session_uuid`),
  FOREIGN KEY (user_id) REFERENCES users(user_id) ON DELETE CASCADE
);