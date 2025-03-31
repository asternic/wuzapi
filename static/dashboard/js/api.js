// API Communication Module

// Configuração global da API
let apiConfig = {
    baseUrl: window.location.origin,
    debug: false // Ativar/desativar logs de depuração
};

/**
 * Definir a URL base da API
 * @param {string} url - URL base para API
 */
function setApiBaseUrl(url) {
    if (url) {
        apiConfig.baseUrl = url;
        localStorage.setItem('wuzapiBaseUrl', url);
        return true;
    }
    return false;
}

/**
 * Obter a URL base da API salva ou padrão
 * @returns {string} URL base da API
 */
function getApiBaseUrl() {
    const savedUrl = localStorage.getItem('wuzapiBaseUrl');
    if (savedUrl) {
        apiConfig.baseUrl = savedUrl;
    }
    return apiConfig.baseUrl;
}

/**
 * Configurar modo de depuração
 * @param {boolean} enabled - Ativar ou desativar depuração
 */
function setDebugMode(enabled) {
    apiConfig.debug = !!enabled;
    if (apiConfig.debug) {
        console.log('API Debug mode enabled');
    }
}

/**
 * Função principal para fazer chamadas à API
 * @param {string} endpoint - Endpoint da API
 * @param {string} method - Método HTTP (GET, POST, etc)
 * @param {object} body - Corpo da requisição para POST/PUT
 * @param {string} specificInstanceId - ID específico da instância para essa chamada
 * @returns {Promise<object>} Resposta da API em formato JSON
 */
async function makeApiCall(endpoint, method = 'GET', body = null, specificInstanceId = null) {
    // Processar endpoints de instância específica
    let instanceId = specificInstanceId;
    
    // Se o endpoint já inclui /instances/{id}/, extrair o ID
    if (!instanceId && endpoint.includes('/instances/')) {
        const match = endpoint.match(/\/instances\/([^\/]+)/);
        if (match && match[1] && match[1] !== 'create') {
            instanceId = match[1];
            // Armazenar o ID para uso no cabeçalho
            window.currentInstanceForRequest = instanceId;
        }
    } else if (!instanceId) {
        // Não é um endpoint de instância específica, usar o ID atual se disponível
        instanceId = window.currentInstanceForRequest || 
                     window.currentInstance || 
                     (instancesState && instancesState.currentInstance) || 
                     null;
    }
    
    // Para usuários não-admin, verificar se o instanceId corresponde ao userId
    if (!window.authState?.isAdmin && window.authState?.userId && 
        instanceId && instanceId !== window.authState.userId.toString()) {
        console.warn('Tentativa de acessar instância não autorizada:', instanceId);
        addLog('Tentativa de acesso não autorizado a instância', 'error');
        return {
            success: false,
            error: 'Não autorizado a acessar esta instância',
            isAuthError: true
        };
    }
    
    // Substituir {id} no endpoint pelo ID da instância atual se existir
    if (instanceId && endpoint.includes('{id}')) {
        endpoint = endpoint.replace('{id}', instanceId);
    }
    
    // Garantir que o endpoint tenha o prefixo API
    if (endpoint.includes('/chat/send/') || endpoint.includes('/messages/')) {
        if (!endpoint.startsWith('/api/v1')) {
            endpoint = `/api/v1${endpoint}`;
        }
    } else if (!endpoint.startsWith('/api/v1') && !endpoint.startsWith('/')) {
        endpoint = `/api/v1/${endpoint}`;
    } else if (!endpoint.startsWith('/api/v1') && endpoint.startsWith('/')) {
        endpoint = `/api/v1${endpoint}`;
    }
    
    const headers = new Headers();
    
    // Obter token e instância atual - garantir que esteja sincronizado
    if (typeof syncAuthToken === 'function') {
        syncAuthToken();
    }
    
    const userToken = window.userToken || localStorage.getItem('wuzapiToken') || '';
    
    if (!userToken) {
        console.error("[AUTH] Nenhum token disponível para fazer requisição:", endpoint);
        return {
            success: false,
            error: 'Token não disponível',
            isAuthError: true
        };
    }
    
    // CORREÇÃO IMPORTANTE: Usar o formato correto de token baseado no endpoint
    if (endpoint.includes('/admin/')) {
        // Os testes mostraram que para rotas admin, o backend espera 'Authorization' sem 'Bearer'
        headers.append('Authorization', userToken);
        console.log("[AUTH] Usando header 'Authorization' para rota admin:", endpoint);
    } else {
        // Para rotas não-admin, adicionar todos os formatos possíveis para garantir compatibilidade
        headers.append('token', userToken);
        headers.append('Authorization', userToken);  // Sem prefix
        headers.append('Authorization', `Bearer ${userToken}`); // Com prefix Bearer
        console.log("[AUTH] Usando múltiplos headers de autenticação para rota:", endpoint);
    }
    
    // Garantir que Content-Type seja aplicado quando houver body
    if (body) {
        headers.append('Content-Type', 'application/json');
    }
    
    // Adicionar o header Instance-Id se necessário
    if (instanceId && !endpoint.includes('/instances/') && !endpoint.includes('/admin/')) {
        headers.append('Instance-Id', instanceId);
        headers.append('x-instance-id', instanceId); // Formato alternativo
    }
    
    const requestUrl = `${apiConfig.baseUrl}${endpoint}`;
    
    if (apiConfig.debug) {
        console.log(`[AUTH] Requisição: ${method} ${requestUrl}`, {
            headers: Object.fromEntries(headers.entries()),
            body: body || '(sem body)',
            token: userToken ? userToken.substring(0, 10) + '...' : 'não definido'
        });
    }
    
    try {
        // Adicionar timestamp para evitar cache
        const url = new URL(requestUrl);
        if (method === 'GET') {
            url.searchParams.append('_', Date.now());
        }
        
        // Tratamento especial para o endpoint de status
        const isStatusEndpoint = endpoint.includes('/status');
        const maxRetries = isStatusEndpoint ? 2 : 0;
        let retryCount = 0;
        let response;
        
        while (retryCount <= maxRetries) {
            response = await fetch(url.toString(), {
                method,
                headers,
                body: body ? JSON.stringify(body) : null
            });
            
            // Se o status endpoint falhar com 500 ou 404, tentamos novamente
            if (isStatusEndpoint && (response.status === 500 || response.status === 404) && retryCount < maxRetries) {
                retryCount++;
                console.log(`[AUTH] Tentativa ${retryCount} para endpoint de status`);
                await new Promise(resolve => setTimeout(resolve, 1000));
                continue;
            }
            
            break;
        }
        
        // Limpar variável temporária usada para esta requisição
        delete window.currentInstanceForRequest;
        
        // Capturar resposta para debugging antes de processar
        const responseClone = response.clone();
        let responseText = null;
        
        try {
            responseText = await responseClone.text();
            if (apiConfig.debug || response.status !== 200) {
                console.log(`[AUTH] Resposta (${response.status}) para ${endpoint}:`, responseText);
            }
        } catch (err) {
            console.error('[AUTH] Erro ao clonar resposta para debug:', err);
        }
        
        // Tratamento específico para status com erro "No session"
        if (isStatusEndpoint && response.status === 500 && responseText) {
            if (responseText.includes("No session")) {
                console.log("[AUTH] Instância sem sessão ativa, retornando status offline");
                return {
                    success: true,
                    data: {
                        Connected: false,
                        connected: false,
                        LoggedIn: false,
                        loggedIn: false,
                        noSession: true,
                        phone: '',
                        jid: '',
                        isOfflineFallback: true
                    }
                };
            }
        }
        
        // Se o endpoint de status falhar após todas as tentativas, retornar estado padrão offline
        if (isStatusEndpoint && (response.status === 500 || response.status === 404)) {
            return {
                success: true,
                data: {
                    Connected: false,
                    connected: false,
                    LoggedIn: false,
                    loggedIn: false,
                    phone: '',
                    jid: '',
                    isOfflineFallback: true
                }
            };
        }
        
        // Verificar se é erro de autorização
        if (response.status === 401 || response.status === 403) {
            console.error('[AUTH] Erro de autenticação detectado:', {
                endpoint,
                status: response.status,
                responseText: responseText && responseText.substring(0, 100)
            });
            
            // Se for endpoint admin, podemos tentar apenas ignorar
            if (endpoint.includes('/admin/')) {
                return {
                    success: false,
                    error: 'Acesso não autorizado',
                    isAuthError: true,
                    status: response.status
                };
            }
            
            // Para outros endpoints, pode ser token expirado, tentar limpar
            if (typeof window.handleAuthError === 'function') {
                window.handleAuthError();
            }
            
            return {
                success: false,
                error: 'Sessão expirada ou token inválido',
                isAuthError: true,
                status: response.status
            };
        }
        
        // Verificar se a resposta é um erro HTTP
        if (!response.ok) {
            const errorMessage = `HTTP error ${response.status}: ${response.statusText}`;
            console.error('[AUTH] Erro HTTP:', errorMessage, responseText);
            
            // Tentar extrair mensagem de erro do responseText
            let errorResponse = null;
            try {
                if (responseText) {
                    errorResponse = JSON.parse(responseText);
                }
            } catch (e) {}
            
            return {
                success: false,
                error: errorResponse?.error || errorMessage,
                status: response.status,
                rawResponse: responseText
            };
        }
        
        // Obter JSON da resposta (já temos o texto da resposta)
        let responseData;
        try {
            if (responseText) {
                responseData = JSON.parse(responseText);
            } else {
                responseData = await response.json();
            }
            
            // Formatar a resposta para manter consistência
            if (responseData.success === undefined) {
                // Adicionar campo success se não existir
                return {
                    success: true,
                    data: responseData
                };
            }
            
            return responseData;
        } catch (e) {
            console.error('[AUTH] Erro ao processar JSON:', e, responseText);
            
            return {
                success: false,
                error: 'Erro ao processar resposta do servidor',
                parseError: e.message,
                rawResponse: responseText
            };
        }
    } catch (error) {
        console.error('[AUTH] Erro de rede:', error);
        
        if (typeof addLog === 'function') {
            addLog(`Erro na chamada API: ${error.message}`, 'error');
        }
        
        return {
            success: false,
            error: `Erro de conexão: ${error.message}`,
            isNetworkError: true
        };
    }
}

/**
 * Conectar uma instância específica ao WhatsApp
 * @param {string} instanceId - ID da instância
 * @param {Array} events - Eventos para assinar (opcional)
 * @returns {Promise<object>} Resultado da operação
 */
async function connectInstance(instanceId, events = ["Message", "ReadReceipt", "Presence", "ChatPresence"]) {
    if (!instanceId) {
        return { success: false, error: "ID da instância não fornecido" };
    }
    
    try {
        return await makeApiCall(`/instances/${instanceId}/connect`, 'POST', {
            Subscribe: events,
            Immediate: true
        });
    } catch (error) {
        if (apiConfig.debug) {
            console.error("Erro ao conectar instância:", error);
        }
        return { success: false, error: error.message };
    }
}

/**
 * Desconectar uma instância específica do WhatsApp
 * @param {string} instanceId - ID da instância
 * @returns {Promise<object>} Resultado da operação
 */
async function disconnectInstance(instanceId) {
    if (!instanceId) {
        return { success: false, error: "ID da instância não fornecido" };
    }
    
    try {
        return await makeApiCall(`/instances/${instanceId}/disconnect`, 'POST');
    } catch (error) {
        if (apiConfig.debug) {
            console.error("Erro ao desconectar instância:", error);
        }
        return { success: false, error: error.message };
    }
}

/**
 * Fazer logout de uma instância específica do WhatsApp
 * @param {string} instanceId - ID da instância
 * @returns {Promise<object>} Resultado da operação
 */
async function logoutInstance(instanceId) {
    if (!instanceId) {
        return { success: false, error: "ID da instância não fornecido" };
    }
    
    try {
        return await makeApiCall(`/instances/${instanceId}/logout`, 'POST');
    } catch (error) {
        if (apiConfig.debug) {
            console.error("Erro ao fazer logout da instância:", error);
        }
        return { success: false, error: error.message };
    }
}

/**
 * Obter o QR code de uma instância específica
 * @param {string} instanceId - ID da instância
 * @returns {Promise<object>} Resultado da operação com o QR code
 */
async function getInstanceQr(instanceId) {
    if (!instanceId) {
        return { success: false, error: "ID da instância não fornecido" };
    }
    
    try {
        return await makeApiCall(`/instances/${instanceId}/qr`, 'GET');
    } catch (error) {
        if (apiConfig.debug) {
            console.error("Erro ao obter QR code da instância:", error);
        }
        return { success: false, error: error.message };
    }
}

/**
 * Obter o status de uma instância específica, com tratamento de erro aprimorado
 * @param {string} instanceId - ID da instância
 * @returns {Promise<object>} Resultado da operação com o status
 */
async function getInstanceStatus(instanceId) {
    if (!instanceId) {
        return { success: false, error: "ID da instância não fornecido" };
    }
    
    try {
        const response = await makeApiCall(`/instances/${instanceId}/status`, 'GET');
        
        // Se a resposta indicar que não há sessão, tratar como desconectado
        if (response.data && response.data.noSession) {
            return {
                success: true,
                data: {
                    Connected: false,
                    connected: false,
                    LoggedIn: false,
                    loggedIn: false,
                    needsReconnect: true
                }
            };
        }
        
        return response;
    } catch (error) {
        if (apiConfig.debug) {
            console.error("Erro ao obter status da instância:", error);
        }
        
        // Retornar um status offline em caso de erro
        return {
            success: true,
            data: {
                Connected: false,
                connected: false,
                LoggedIn: false,
                loggedIn: false,
                isErrorFallback: true
            }
        };
    }
}

/**
 * Obter informações de uma instância específica
 * @param {string} instanceId - ID da instância
 * @returns {Promise<object>} Dados da instância
 */
async function getInstanceInfo(instanceId) {
    if (!instanceId) {
        return { success: false, error: "ID da instância não fornecido" };
    }
    
    try {
        // Para admin, podemos obter a lista completa e filtrar
        if (window.authState?.isAdmin) {
            const response = await makeApiCall('/instances', 'GET');
            
            if (response.success && Array.isArray(response.data)) {
                const instance = response.data.find(inst => inst.id == instanceId);
                
                if (instance) {
                    return {
                        success: true,
                        data: instance
                    };
                } else {
                    return {
                        success: false,
                        error: "Instância não encontrada"
                    };
                }
            }
            
            return response;
        } else {
            // Para usuário normal, apenas verificar se a instância corresponde ao seu ID
            if (window.authState?.userId && instanceId == window.authState.userId) {
                const response = await makeApiCall('/instances', 'GET');
                
                if (response.success && Array.isArray(response.data) && response.data.length > 0) {
                    return {
                        success: true,
                        data: response.data[0]
                    };
                }
                
                return response;
            } else {
                return {
                    success: false,
                    error: "Não autorizado a acessar esta instância"
                };
            }
        }
    } catch (error) {
        if (apiConfig.debug) {
            console.error("Erro ao obter informações da instância:", error);
        }
        return { success: false, error: error.message };
    }
}

/**
 * Atualizar o nome de uma instância
 * @param {string} instanceId - ID da instância
 * @param {string} name - Novo nome da instância
 * @returns {Promise<object>} Resultado da operação
 */
async function updateInstanceName(instanceId, name) {
    if (!instanceId || !name) {
        return { success: false, error: "ID da instância ou nome não fornecido" };
    }
    
    try {
        return await makeApiCall(`/instances/${instanceId}/update`, 'POST', {
            name: name
        });
    } catch (error) {
        if (apiConfig.debug) {
            console.error("Erro ao atualizar nome da instância:", error);
        }
        return { success: false, error: error.message };
    }
}

/**
 * Criar uma nova instância
 * @param {object} instanceData - Dados da nova instância
 * @returns {Promise<object>} Resultado da operação
 */
async function createInstance(instanceData) {
    if (!instanceData || !instanceData.name) {
        return { success: false, error: "Dados da instância incompletos" };
    }
    
    try {
        // Verificar se o usuário é admin
        if (!window.authState?.isAdmin) {
            return {
                success: false,
                error: "Apenas administradores podem criar instâncias"
            };
        }
        
        return await makeApiCall('/instances/create', 'POST', instanceData);
    } catch (error) {
        if (apiConfig.debug) {
            console.error("Erro ao criar instância:", error);
        }
        return { success: false, error: error.message };
    }
}

/**
 * Excluir uma instância
 * @param {string} instanceId - ID da instância
 * @returns {Promise<object>} Resultado da operação
 */
async function deleteInstance(instanceId) {
    if (!instanceId) {
        return { success: false, error: "ID da instância não fornecido" };
    }
    
    try {
        // Verificar se o usuário é admin
        if (!window.authState?.isAdmin) {
            return {
                success: false,
                error: "Apenas administradores podem excluir instâncias"
            };
        }
        
        return await makeApiCall(`/instances/${instanceId}/delete`, 'DELETE');
    } catch (error) {
        if (apiConfig.debug) {
            console.error("Erro ao excluir instância:", error);
        }
        return { success: false, error: error.message };
    }
}

// Inicialização
document.addEventListener('DOMContentLoaded', () => {
    // Carregar URL da API salva
    getApiBaseUrl();
    
    // Verificar parâmetro de depuração na URL
    const urlParams = new URLSearchParams(window.location.search);
    if (urlParams.has('debug') && urlParams.get('debug') === 'true') {
        setDebugMode(true);
    }
}); 