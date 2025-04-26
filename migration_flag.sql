-- Adicionando flag is_official nas tabelas principais
ALTER TABLE contacts
ADD COLUMN is_official BOOLEAN DEFAULT true;

ALTER TABLE messages
ADD COLUMN is_official BOOLEAN DEFAULT true;

ALTER TABLE mensagens_agendadas
ADD COLUMN is_official BOOLEAN DEFAULT true;

-- Adicionando flag is_official na tabela setores
ALTER TABLE setores
ADD COLUMN is_official BOOLEAN DEFAULT true;

-- Índices para melhorar performance de consultas por tipo
CREATE INDEX idx_contacts_official ON contacts(is_official);
CREATE INDEX idx_messages_official ON messages(is_official);
CREATE INDEX idx_agendadas_official ON mensagens_agendadas(is_official);
CREATE INDEX idx_setores_official ON setores(is_official);

-- Criando tabela para gerenciar conexões do WhatsApp não oficial
CREATE TABLE whatsapp_connections (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    setor_id INT NOT NULL,
    status ENUM('disconnected', 'connecting', 'connected') NOT NULL DEFAULT 'disconnected',
    qrcode_base64 TEXT,
    last_qrcode_generated_at TIMESTAMP NULL,
    last_connected_at TIMESTAMP NULL,
    last_disconnected_at TIMESTAMP NULL,
    session_data TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (setor_id) REFERENCES setores(id) ON DELETE CASCADE ON UPDATE CASCADE,
    CONSTRAINT unique_setor_connection UNIQUE (setor_id)
);

-- Índice para busca por status
CREATE INDEX idx_whatsapp_connections_status ON whatsapp_connections(status); 