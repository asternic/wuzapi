# Despliegue de WuzAPI en EasyPanel

Esta guia explica como desplegar WuzAPI en [EasyPanel](https://easypanel.io).

## Requisitos Previos

- Una instancia de EasyPanel funcionando (v1.0+)
- Acceso al panel de administracion de EasyPanel
- Un dominio configurado (opcional pero recomendado)

## Metodo 1: Despliegue con Docker Compose (Recomendado)

### Paso 1: Crear un nuevo proyecto

1. Accede a tu panel de EasyPanel
2. Haz clic en **"Create Project"**
3. Asigna un nombre al proyecto, por ejemplo: `wuzapi`

### Paso 2: Agregar el servicio

1. Dentro del proyecto, haz clic en **"+ Service"**
2. Selecciona **"Docker Compose"**
3. Copia el contenido de `docker-compose-easypanel.yml` o apunta al repositorio

### Paso 3: Configurar variables de entorno

En la seccion de **Environment Variables** de EasyPanel, configura:

#### Variables Requeridas

```env
# Token de administrador (genera uno seguro)
WUZAPI_ADMIN_TOKEN=tu_token_seguro_aqui_32_caracteres

# Clave de encriptacion (exactamente 32 caracteres para AES-256)
WUZAPI_GLOBAL_ENCRYPTION_KEY=tu_clave_encriptacion_32_chars

# Clave HMAC para firmar webhooks (minimo 32 caracteres)
WUZAPI_GLOBAL_HMAC_KEY=tu_clave_hmac_minimo_32_caracteres

# Password de la base de datos
DB_PASSWORD=tu_password_seguro_postgres
```

#### Variables Opcionales

```env
# Webhook global para recibir todos los eventos
WUZAPI_GLOBAL_WEBHOOK=https://tu-servidor.com/webhook

# Zona horaria
TZ=America/Sao_Paulo

# Formato de webhook: json o form
WEBHOOK_FORMAT=json

# Nombre del dispositivo en WhatsApp
SESSION_DEVICE_NAME=WuzAPI

# Configuracion de reintentos de webhook
WEBHOOK_RETRY_ENABLED=true
WEBHOOK_RETRY_COUNT=5
WEBHOOK_RETRY_DELAY_SECONDS=30

# Password de RabbitMQ (opcional)
RABBITMQ_PASSWORD=tu_password_rabbitmq
```

### Paso 4: Configurar dominio

1. Ve a la seccion **"Domains"** del servicio `wuzapi`
2. Agrega tu dominio: `wuzapi.tudominio.com`
3. EasyPanel configurara automaticamente SSL con Let's Encrypt

### Paso 5: Desplegar

1. Haz clic en **"Deploy"**
2. Espera a que todos los servicios inicien (puede tomar 2-3 minutos)
3. Verifica el estado en la seccion **"Logs"**

## Metodo 2: Despliegue con Template de EasyPanel

Si prefieres usar el archivo `easypanel.json`:

1. En EasyPanel, ve a **Settings > Templates**
2. Haz clic en **"Import Template"**
3. Pega el contenido de `easypanel.json`
4. Guarda y despliega desde la seccion de templates

## Metodo 3: Despliegue desde GitHub

1. Crea un nuevo servicio en EasyPanel
2. Selecciona **"GitHub"** como fuente
3. Conecta tu repositorio de WuzAPI
4. Selecciona la rama `main`
5. EasyPanel detectara automaticamente el `Dockerfile`
6. Configura las variables de entorno como se indica arriba

## Estructura de Servicios

El despliegue incluye tres servicios:

| Servicio | Puerto | Descripcion |
|----------|--------|-------------|
| `wuzapi` | 8080 | API principal de WhatsApp |
| `db` | 5432 | PostgreSQL para datos persistentes |
| `rabbitmq` | 5672/15672 | Cola de mensajes (opcional) |

## Acceso a los Servicios

### API de WuzAPI
- URL: `https://wuzapi.tudominio.com`
- Dashboard: `https://wuzapi.tudominio.com/dashboard`
- Documentacion API: `https://wuzapi.tudominio.com/api`
- Login QR: `https://wuzapi.tudominio.com/login`

### RabbitMQ Management (opcional)
- URL: `https://rabbitmq.tudominio.com` (si configuraste el subdominio)
- Usuario: `wuzapi`
- Password: El valor de `RABBITMQ_PASSWORD`

## Verificar el Despliegue

1. Accede a `https://wuzapi.tudominio.com/api`
2. Deberias ver la documentacion de Swagger
3. Prueba el endpoint de health: `GET /session/status`

## Persistencia de Datos

Los siguientes volumenes se crean automaticamente:

- `wuzapi_data`: Datos de sesion de WhatsApp
- `wuzapi_postgres_data`: Base de datos PostgreSQL
- `wuzapi_rabbitmq_data`: Datos de RabbitMQ

**Importante**: Realiza backups periodicos de estos volumenes.

## Escalado

Para escalar WuzAPI horizontalmente:

1. Ve a la configuracion del servicio `wuzapi`
2. En **"Deploy"**, ajusta el numero de replicas
3. Nota: Cada replica necesita su propia sesion de WhatsApp

## Monitoreo

EasyPanel proporciona:

- **Logs**: Ver logs en tiempo real de cada servicio
- **Metricas**: CPU, memoria y red
- **Alertas**: Configura notificaciones para caidas

## Solucion de Problemas

### El servicio no inicia

1. Verifica los logs en EasyPanel
2. Asegurate de que las variables de entorno esten configuradas
3. Verifica que el puerto 8080 no este en uso

### Error de conexion a la base de datos

1. Espera a que el servicio `db` este saludable
2. Verifica que `DB_HOST=db` este configurado
3. Revisa los logs de PostgreSQL

### No se puede escanear el QR

1. Verifica que el servicio este corriendo
2. Accede a `https://wuzapi.tudominio.com/login`
3. Revisa los logs para errores de WhatsApp

### Webhook no funciona

1. Verifica que `WUZAPI_GLOBAL_WEBHOOK` este configurado
2. Asegurate de que la URL sea accesible desde el servidor
3. Revisa los logs para errores de conexion

## Actualizaciones

Para actualizar WuzAPI:

1. En EasyPanel, ve al servicio `wuzapi`
2. Haz clic en **"Rebuild"**
3. EasyPanel descargara la ultima version y reiniciara el servicio

## Recursos

- [Documentacion de WuzAPI](https://github.com/asternic/wuzapi)
- [Documentacion de EasyPanel](https://easypanel.io/docs)
- [API Reference](/api)

## Soporte

Si encuentras problemas:

1. Revisa los [issues de GitHub](https://github.com/asternic/wuzapi/issues)
2. Consulta la [documentacion de EasyPanel](https://easypanel.io/docs)
3. Abre un nuevo issue con los logs relevantes
