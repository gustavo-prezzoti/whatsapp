-- Criar tabela de configuração do WhatsApp
CREATE TABLE whatsapp_config (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    type ENUM('official', 'unofficial') NOT NULL,
    name VARCHAR(100) NOT NULL,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

-- Adicionar referência nas tabelas principais
ALTER TABLE contacts
ADD COLUMN whatsapp_config_id BIGINT,
ADD FOREIGN KEY (whatsapp_config_id) REFERENCES whatsapp_config(id);

ALTER TABLE messages
ADD COLUMN whatsapp_config_id BIGINT,
ADD FOREIGN KEY (whatsapp_config_id) REFERENCES whatsapp_config(id);

ALTER TABLE mensagens_agendadas
ADD COLUMN whatsapp_config_id BIGINT,
ADD FOREIGN KEY (whatsapp_config_id) REFERENCES whatsapp_config(id);

-- Inserir configurações iniciais
INSERT INTO whatsapp_config (type, name) VALUES
('official', 'WhatsApp Oficial'),
('unofficial', 'WhatsApp Não Oficial');

-- Índices para melhorar performance
CREATE INDEX idx_contacts_config ON contacts(whatsapp_config_id);
CREATE INDEX idx_messages_config ON messages(whatsapp_config_id);
CREATE INDEX idx_agendadas_config ON mensagens_agendadas(whatsapp_config_id); 