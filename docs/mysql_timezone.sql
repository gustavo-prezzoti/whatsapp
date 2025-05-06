-- Configurar o fuso horário global do MySQL para Brasília
SET GLOBAL time_zone = 'America/Sao_Paulo';
SET time_zone = 'America/Sao_Paulo';

-- Para tornar a configuração permanente, edite o arquivo my.cnf ou my.ini do MySQL e adicione:
-- [mysqld]
-- default-time-zone = 'America/Sao_Paulo'

-- Verificar a configuração
SELECT @@global.time_zone, @@session.time_zone; 