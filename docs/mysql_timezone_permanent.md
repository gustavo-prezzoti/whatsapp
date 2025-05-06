# Configurando o fuso horário do MySQL permanentemente no Ubuntu (AWS)

Para configurar permanentemente o fuso horário do MySQL para Brasília no Ubuntu, siga estes passos:

## 1. Editar o arquivo de configuração do MySQL

```bash
sudo nano /etc/mysql/mysql.conf.d/mysqld.cnf
```

## 2. Adicionar a configuração de timezone

Adicione a seguinte linha na seção `[mysqld]`:

```
default-time-zone = 'America/Sao_Paulo'
```

## 3. Salvar o arquivo e sair

No nano, pressione `Ctrl+O` e depois `Enter` para salvar, e `Ctrl+X` para sair.

## 4. Reiniciar o serviço MySQL

```bash
sudo systemctl restart mysql
```

## 5. Verificar se a configuração foi aplicada

```bash
mysql -e "SELECT @@global.time_zone, @@session.time_zone;"
```

Deve mostrar:
```
+--------------------+--------------------+
| @@global.time_zone | @@session.time_zone |
+--------------------+--------------------+
| America/Sao_Paulo  | America/Sao_Paulo  |
+--------------------+--------------------+
```

## Solução de problemas

Se você receber um erro relacionado a timezones não carregados, pode ser necessário carregar as tabelas de timezone:

```bash
mysql_tzinfo_to_sql /usr/share/zoneinfo | mysql -u root -p mysql
``` 