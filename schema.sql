-- Criação do banco de dados
CREATE DATABASE IF NOT EXISTS ligchat_unofficial;
USE ligchat_unofficial;

-- Tabela de contatos
CREATE TABLE contacts (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    name VARCHAR(255),
    number VARCHAR(20) NOT NULL,
    avatar_url TEXT,
    sector_id BIGINT,
    tag_id BIGINT,
    is_active BOOLEAN DEFAULT true,
    email VARCHAR(255),
    notes TEXT,
    ai_active BOOLEAN DEFAULT false,
    assigned_to BIGINT,
    priority VARCHAR(50),
    contact_status VARCHAR(50),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

-- Tabela de mensagens
CREATE TABLE messages (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    conteudo TEXT,
    tipo VARCHAR(50),
    url TEXT,
    nome_arquivo VARCHAR(255),
    mime_type VARCHAR(100),
    id_setor BIGINT,
    contato_id BIGINT,
    data_envio TIMESTAMP,
    enviado BOOLEAN DEFAULT false,
    lido BOOLEAN DEFAULT false,
    WhatsAppMessageId VARCHAR(100)
);

-- Tabela de mensagens agendadas
CREATE TABLE mensagens_agendadas (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    nome VARCHAR(255),
    mensagem_de_texto TEXT,
    data_envio TIMESTAMP,
    setor_id BIGINT,
    data_criacao TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    data_atualizacao TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    tag_id BIGINT,
    contato_id BIGINT,
    status VARCHAR(50)
);

-- Tabela de usuários
CREATE TABLE usuarios (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    nome VARCHAR(255),
    email VARCHAR(255),
    avatar_url TEXT,
    phone_whatsapp VARCHAR(20),
    setor_id BIGINT,
    data_criacao TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    data_atualizacao TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    status VARCHAR(50),
    is_admin BOOLEAN DEFAULT false,
    verification_code VARCHAR(6),
    verification_code_expires_at TIMESTAMP,
    invited_by BIGINT
);

-- Índices para melhor performance
CREATE INDEX idx_contacts_number ON contacts(number);
CREATE INDEX idx_messages_contato ON messages(contato_id);
CREATE INDEX idx_mensagens_agendadas_contato ON mensagens_agendadas(contato_id);
CREATE INDEX idx_usuarios_email ON usuarios(email); 