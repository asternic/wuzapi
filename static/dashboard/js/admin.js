// Admin Dashboard Module

/**
 * Inicializar o painel de administração
 */
function initAdminPanel() {
    // Registrar eventos de administrador
    registerAdminEvents();
    
    // Mostrar todos os elementos administrativos
    document.querySelectorAll('.admin-only').forEach(el => {
        el.style.display = '';
    });
    
    // Garantir que também veja os elementos de usuário para gerenciamento
    document.querySelectorAll('.user-only').forEach(el => {
        el.style.display = '';
    });
    
    // Carregar estatísticas do sistema
    loadAdminStats();
    
    // Garantir visibilidade do painel admin
    const adminDashboardPanel = document.getElementById('adminDashboardPanel');
    if (adminDashboardPanel) {
        adminDashboardPanel.style.display = '';
    }
    
    // Garantir que a sidebar esteja visível
    const sidebarCol = document.querySelector('.col-md-3.col-lg-2');
    if (sidebarCol) {
        sidebarCol.classList.remove('d-none');
    }
    
    // Garantir que o conteúdo principal tenha tamanho apropriado
    const mainCol = document.querySelector('.col-md-9.col-lg-10');
    if (mainCol) {
        mainCol.className = 'col-md-9 col-lg-10';
    }
    
    // Garantir que o badge de admin esteja visível
    const adminBadge = document.getElementById('adminBadge');
    if (adminBadge) {
        adminBadge.classList.remove('d-none');
    }
    
    // Garantir que todas as seções estejam visíveis
    document.querySelectorAll('.dashboard-section').forEach(section => {
        section.style.display = '';
    });
    
    // Permitir ver todas as instâncias
    const manageInstancesLink = document.querySelector('.nav-link[data-bs-toggle="modal"][data-bs-target="#manageInstancesModal"]');
    if (manageInstancesLink) {
        manageInstancesLink.style.display = '';
    }
    
    // Permitir adicionar novas instâncias
    const addInstanceButtons = document.querySelectorAll('[data-bs-target="#addInstanceModal"]');
    if (addInstanceButtons) {
        addInstanceButtons.forEach(btn => {
            btn.style.display = '';
        });
    }
    
    // Garantir que botão de verificar sistema esteja correto
    const checkSystemStatusBtn = document.getElementById('checkSystemStatusBtn');
    if (checkSystemStatusBtn) {
        checkSystemStatusBtn.innerHTML = '<i class="bi bi-check-circle"></i>Verificar Sistema';
        checkSystemStatusBtn.setAttribute('data-function', 'check-system-status');
        
        // Remover evento antigo se existir
        checkSystemStatusBtn.removeEventListener('click', checkInstanceStatus);
        // Adicionar evento para admin
        checkSystemStatusBtn.addEventListener('click', checkSystemStatus);
    }
}

/**
 * Registrar eventos para o painel de administração
 */
function registerAdminEvents() {
    // Botão de atualizar estatísticas
    const refreshAdminStats = document.getElementById('refreshAdminStats');
    if (refreshAdminStats) {
        refreshAdminStats.addEventListener('click', loadAdminStats);
    }
    
    // Botão de verificar sistema
    const checkSystemBtn = document.getElementById('checkSystemBtn');
    if (checkSystemBtn) {
        checkSystemBtn.addEventListener('click', checkSystemStatus);
    }
    
    // Botão de visualizar todas as instâncias
    const adminViewAllBtn = document.getElementById('adminViewAllBtn');
    if (adminViewAllBtn) {
        adminViewAllBtn.addEventListener('click', () => {
            // Focar no painel de lista de instâncias
            const adminInstancesListPanel = document.getElementById('adminInstancesListPanel');
            if (adminInstancesListPanel) {
                adminInstancesListPanel.scrollIntoView({ behavior: 'smooth' });
            }
            // Atualizar a lista
            updateAdminInstancesList();
        });
    }
    
    // Botão de atualizar lista de instâncias
    const refreshInstancesListBtn = document.getElementById('refreshInstancesListBtn');
    if (refreshInstancesListBtn) {
        refreshInstancesListBtn.addEventListener('click', updateAdminInstancesList);
    }
    
    // Botão de enviar mensagem de teste
    const adminSendTestBtn = document.getElementById('adminSendTestBtn');
    if (adminSendTestBtn) {
        adminSendTestBtn.addEventListener('click', sendTestMessage);
    }
    
    // Botão de atualizar status do sistema
    const refreshSystemStatusBtn = document.getElementById('refreshSystemStatusBtn');
    if (refreshSystemStatusBtn) {
        refreshSystemStatusBtn.addEventListener('click', () => {
            loadAdminStats();
            showToast('Status do sistema atualizado', 'success');
        });
    }
}

/**
 * Carregar estatísticas para o painel administrativo
 */
async function loadAdminStats() {
    if (!authState?.isAdmin) return;
    
    try {
        addLog('Carregando estatísticas de administrador...', 'info');
        showLoading('Atualizando painel administrativo...');
        
        // Carregar lista de instâncias através da API
        const instancesResponse = await makeApiCall('/instances', 'GET');
        
        let instances = [];
        if (instancesResponse.success && Array.isArray(instancesResponse.data)) {
            instances = instancesResponse.data;
            
            // Contar instâncias por status
            const totalInstances = instances.length;
            let connectedCount = 0;
            let disconnectedCount = 0;
            
            // Atualizar o dropdown de instâncias
            const adminTestInstance = document.getElementById('adminTestInstance');
            if (adminTestInstance) {
                // Limpar opções existentes
                adminTestInstance.innerHTML = '<option value="">Selecione uma instância</option>';
                
                // Contar conectados/desconectados e adicionar ao dropdown
                instances.forEach(instance => {
                    const isConnected = instance.connected === 1 || instance.connected === true;
                    if (isConnected) {
                        connectedCount++;
                    } else {
                        disconnectedCount++;
                    }
                    
                    // Adicionar ao dropdown
                    const option = document.createElement('option');
                    option.value = instance.id;
                    option.textContent = `${instance.name || 'Instância ' + instance.id} ${instance.phone ? '(' + instance.phone + ')' : ''}`;
                    option.dataset.phone = instance.phone || '';
                    option.dataset.jid = instance.jid || '';
                    adminTestInstance.appendChild(option);
                });
                
                // Atualizar contadores
                const totalInstancesEl = document.getElementById('totalInstances');
                const connectedInstancesEl = document.getElementById('connectedInstances');
                const disconnectedInstancesEl = document.getElementById('disconnectedInstances');
                
                const totalInstancesCountEl = document.getElementById('totalInstancesCount');
                const connectedInstancesCountEl = document.getElementById('connectedInstancesCount');
                const disconnectedInstancesCountEl = document.getElementById('disconnectedInstancesCount');
                
                if (totalInstancesEl) {
                    totalInstancesEl.textContent = totalInstances;
                    totalInstancesEl.classList.add('fade-in');
                    setTimeout(() => totalInstancesEl.classList.remove('fade-in'), 500);
                }
                
                if (connectedInstancesEl) {
                    connectedInstancesEl.textContent = connectedCount;
                    connectedInstancesEl.classList.add('fade-in');
                    setTimeout(() => connectedInstancesEl.classList.remove('fade-in'), 500);
                }
                
                if (disconnectedInstancesEl) {
                    disconnectedInstancesEl.textContent = disconnectedCount;
                    disconnectedInstancesEl.classList.add('fade-in');
                    setTimeout(() => disconnectedInstancesEl.classList.remove('fade-in'), 500);
                }
                
                // Atualizar contadores compactos
                if (totalInstancesCountEl) totalInstancesCountEl.textContent = totalInstances;
                if (connectedInstancesCountEl) connectedInstancesCountEl.textContent = connectedCount;
                if (disconnectedInstancesCountEl) disconnectedInstancesCountEl.textContent = disconnectedCount;
            }
            
            // Preencher a tabela de instâncias
            updateAdminInstancesList(instances);
        } else {
            addLog('Falha ao carregar lista de instâncias', 'error');
        }
        
        hideLoading();
        addLog('Estatísticas de administração atualizadas', 'success');
    } catch (error) {
        hideLoading();
        addLog('Erro ao carregar estatísticas de administração', 'error');
        console.error('Erro ao carregar estatísticas:', error);
    }
}

/**
 * Atualiza a lista de instâncias na tabela de administração
 * @param {Array} instances - Lista de instâncias (opcional, se não fornecido carrega novamente)
 */
async function updateAdminInstancesList(instances = null) {
    if (!authState?.isAdmin) return;
    
    try {
        const tableBody = document.getElementById('adminInstancesList');
        if (!tableBody) return;
        
        // Se não recebeu instâncias, carregá-las
        if (!instances) {
            tableBody.innerHTML = '<tr><td colspan="5" class="text-center"><div class="spinner-border spinner-border-sm text-primary me-2"></div>Carregando instâncias...</td></tr>';
            
            const response = await makeApiCall('/instances', 'GET');
            if (response.success && Array.isArray(response.data)) {
                instances = response.data;
            } else {
                tableBody.innerHTML = '<tr><td colspan="5" class="text-center text-danger">Erro ao carregar instâncias</td></tr>';
                return;
            }
        }
        
        // Atualizar contador
        const instancesCount = document.getElementById('instancesCount');
        if (instancesCount) {
            instancesCount.textContent = instances.length;
        }
        
        // Se não há instâncias, mostrar mensagem
        if (instances.length === 0) {
            tableBody.innerHTML = '<tr><td colspan="5" class="text-center">Nenhuma instância encontrada</td></tr>';
            return;
        }
        
        // Limpar tabela
        tableBody.innerHTML = '';
        
        // Adicionar cada instância à tabela
        instances.forEach(instance => {
            const row = document.createElement('tr');
            
            // Verificar status
            const isConnected = instance.connected === 1 || instance.connected === true;
            const isLoggedIn = instance.loggedIn === true;
            
            // Classe para destacar linha baseada no status
            if (isConnected && isLoggedIn) {
                row.classList.add('table-success');
            } else if (isConnected) {
                row.classList.add('table-warning');
            } else {
                row.classList.add('table-danger');
            }
            
            // Definir ID para referência
            row.setAttribute('data-instance-id', instance.id);
            
            // Status visual
            let statusHtml = '';
            if (isConnected && isLoggedIn) {
                statusHtml = '<span class="badge bg-success">Conectado</span>';
            } else if (isConnected) {
                statusHtml = '<span class="badge bg-warning">Aguardando QR</span>';
            } else {
                statusHtml = '<span class="badge bg-danger">Desconectado</span>';
            }
            
            // Conteúdo da linha
            row.innerHTML = `
                <td>${instance.id}</td>
                <td>${instance.name || `Instância ${instance.id}`}</td>
                <td>${instance.phone || '<span class="text-muted">Não disponível</span>'}</td>
                <td>${statusHtml}</td>
                <td>
                    <div class="btn-group btn-group-sm">
                        <button class="btn btn-outline-primary select-instance-btn" data-instance-id="${instance.id}" title="Selecionar">
                            <i class="bi bi-check-lg"></i>
                        </button>
                        ${isConnected ? 
                            `<button class="btn btn-outline-warning disconnect-instance-btn" data-instance-id="${instance.id}" title="Desconectar">
                                <i class="bi bi-pause-circle"></i>
                             </button>` 
                            : 
                            `<button class="btn btn-outline-success connect-instance-btn" data-instance-id="${instance.id}" title="Conectar">
                                <i class="bi bi-play-circle"></i>
                             </button>`
                        }
                        <button class="btn btn-outline-danger delete-instance-btn" data-instance-id="${instance.id}" title="Excluir">
                            <i class="bi bi-trash"></i>
                        </button>
                    </div>
                </td>
            `;
            
            // Adicionar à tabela
            tableBody.appendChild(row);
        });
        
        // Adicionar eventos aos botões
        tableBody.querySelectorAll('.select-instance-btn').forEach(btn => {
            btn.addEventListener('click', () => {
                const instanceId = btn.getAttribute('data-instance-id');
                if (typeof selectInstance === 'function') {
                    selectInstance(instanceId);
                    
                    // Esconder painel admin e mostrar instância
                    const adminDashboardPanel = document.getElementById('adminDashboardPanel');
                    const adminInstancesListPanel = document.getElementById('adminInstancesListPanel');
                    
                    if (adminDashboardPanel) adminDashboardPanel.style.display = 'none';
                    if (adminInstancesListPanel) adminInstancesListPanel.style.display = 'none';
                    
                    showToast(`Instância ${instanceId} selecionada`, 'success');
                }
            });
        });
        
        tableBody.querySelectorAll('.connect-instance-btn').forEach(btn => {
            btn.addEventListener('click', async () => {
                const instanceId = btn.getAttribute('data-instance-id');
                
                try {
                    showLoading(`Conectando instância ${instanceId}...`);
                    const response = await connectInstance(instanceId);
                    hideLoading();
                    
                    if (response.success) {
                        showToast(`Instância ${instanceId} conectada com sucesso`, 'success');
                        addLog(`Instância ${instanceId} conectada com sucesso`, 'success');
                        
                        // Atualizar a lista
                        updateAdminInstancesList();
                    } else {
                        showToast(`Erro ao conectar instância: ${response.error}`, 'danger');
                        addLog(`Erro ao conectar instância ${instanceId}: ${response.error}`, 'error');
                    }
                } catch (error) {
                    hideLoading();
                    showToast('Erro ao conectar instância', 'danger');
                    addLog(`Erro ao conectar instância ${instanceId}: ${error.message}`, 'error');
                }
            });
        });
        
        tableBody.querySelectorAll('.disconnect-instance-btn').forEach(btn => {
            btn.addEventListener('click', async () => {
                const instanceId = btn.getAttribute('data-instance-id');
                
                try {
                    showLoading(`Desconectando instância ${instanceId}...`);
                    const response = await disconnectInstance(instanceId);
                    hideLoading();
                    
                    if (response.success) {
                        showToast(`Instância ${instanceId} desconectada com sucesso`, 'success');
                        addLog(`Instância ${instanceId} desconectada com sucesso`, 'success');
                        
                        // Atualizar a lista
                        updateAdminInstancesList();
                    } else {
                        showToast(`Erro ao desconectar instância: ${response.error}`, 'danger');
                        addLog(`Erro ao desconectar instância ${instanceId}: ${response.error}`, 'error');
                    }
                } catch (error) {
                    hideLoading();
                    showToast('Erro ao desconectar instância', 'danger');
                    addLog(`Erro ao desconectar instância ${instanceId}: ${error.message}`, 'error');
                }
            });
        });
        
        tableBody.querySelectorAll('.delete-instance-btn').forEach(btn => {
            btn.addEventListener('click', async () => {
                const instanceId = btn.getAttribute('data-instance-id');
                
                if (confirm(`Tem certeza que deseja excluir a instância ${instanceId}? Esta ação não pode ser desfeita.`)) {
                    try {
                        showLoading(`Excluindo instância ${instanceId}...`);
                        const response = await deleteInstance(instanceId);
                        hideLoading();
                        
                        if (response.success) {
                            showToast(`Instância ${instanceId} excluída com sucesso`, 'success');
                            addLog(`Instância ${instanceId} excluída com sucesso`, 'success');
                            
                            // Atualizar a lista
                            updateAdminInstancesList();
                            // Atualizar estatísticas
                            loadAdminStats();
                            
                            // Se a instância excluída era a atual, selecionar outra
                            if (instancesState.currentInstance == instanceId) {
                                instancesState.currentInstance = null;
                                if (instancesState.instances.length > 0) {
                                    // Encontrar a primeira instância diferente da excluída
                                    const nextInstance = instancesState.instances.find(i => i.id != instanceId);
                                    if (nextInstance) {
                                        selectInstance(nextInstance.id);
                                    }
                                }
                            }
                        } else {
                            showToast(`Erro ao excluir instância: ${response.error}`, 'danger');
                            addLog(`Erro ao excluir instância ${instanceId}: ${response.error}`, 'error');
                        }
                    } catch (error) {
                        hideLoading();
                        showToast('Erro ao excluir instância', 'danger');
                        addLog(`Erro ao excluir instância ${instanceId}: ${error.message}`, 'error');
                    }
                }
            });
        });
        
    } catch (error) {
        console.error('Erro ao atualizar lista de instâncias:', error);
        addLog('Erro ao atualizar lista de instâncias', 'error');
    }
}

/**
 * Verificar o status do sistema (para administrador)
 */
async function checkSystemStatus() {
    if (!authState?.isAdmin) {
        addLog('Apenas administradores podem verificar o status do sistema', 'warning');
        return;
    }
    
    try {
        showLoading('Verificando status do sistema...');
        addLog('Verificando status do sistema...', 'info');
        
        // Carregar lista de instâncias
        const instancesResponse = await makeApiCall('/instances', 'GET');
        
        if (!instancesResponse.success || !Array.isArray(instancesResponse.data)) {
            hideLoading();
            addLog('Falha ao obter lista de instâncias', 'error');
            showToast('Falha ao verificar status do sistema', 'danger');
            return;
        }
        
        const instances = instancesResponse.data;
        let totalConnected = 0;
        let totalDisconnected = 0;
        
        // Criar um log dos resultados
        let statusLog = '<div class="system-status-log">';
        statusLog += '<h5>Relatório de Status do Sistema</h5>';
        statusLog += `<p>Total de instâncias: ${instances.length}</p>`;
        statusLog += '<table class="table table-sm table-striped"><thead><tr><th>ID</th><th>Nome</th><th>Status</th></tr></thead><tbody>';
        
        // Verificar cada instância
        for (const instance of instances) {
            try {
                const statusResponse = await makeApiCall(`/instances/${instance.id}/status`, 'GET');
                
                let statusText = '';
                let statusClass = '';
                
                if (statusResponse.success && statusResponse.data) {
                    const data = statusResponse.data;
                    
                    const isConnected = data.Connected === true || data.connected === true;
                    const isLoggedIn = data.LoggedIn === true || data.loggedIn === true;
                    
                    if (isConnected && isLoggedIn) {
                        statusText = 'Conectado e Autenticado';
                        statusClass = 'text-success';
                        totalConnected++;
                    } else if (isConnected) {
                        statusText = 'Conectado (aguardando QR)';
                        statusClass = 'text-warning';
                        totalConnected++;
                    } else {
                        statusText = 'Desconectado';
                        statusClass = 'text-danger';
                        totalDisconnected++;
                    }
                } else {
                    statusText = 'Erro ao verificar';
                    statusClass = 'text-danger';
                    totalDisconnected++;
                }
                
                statusLog += `<tr><td>${instance.id}</td><td>${instance.name || 'Sem nome'}</td><td class="${statusClass}">${statusText}</td></tr>`;
            } catch (error) {
                statusLog += `<tr><td>${instance.id}</td><td>${instance.name || 'Sem nome'}</td><td class="text-danger">Erro: ${error.message}</td></tr>`;
                totalDisconnected++;
            }
        }
        
        statusLog += '</tbody></table>';
        statusLog += `<p>Conectadas: ${totalConnected} | Desconectadas: ${totalDisconnected}</p>`;
        
        // Adicionar timestamp
        statusLog += `<p class="text-muted small">Verificação concluída em ${new Date().toLocaleString()}</p>`;
        statusLog += '</div>';
        
        hideLoading();
        
        // Mostrar resultados em um modal
        const modalHtml = `
            <div class="modal fade" id="systemStatusModal" tabindex="-1" aria-hidden="true">
                <div class="modal-dialog modal-lg">
                    <div class="modal-content">
                        <div class="modal-header">
                            <h5 class="modal-title">Status do Sistema</h5>
                            <button type="button" class="btn-close" data-bs-dismiss="modal" aria-label="Close"></button>
                        </div>
                        <div class="modal-body">
                            ${statusLog}
                        </div>
                        <div class="modal-footer">
                            <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">Fechar</button>
                        </div>
                    </div>
                </div>
            </div>
        `;
        
        const modalContainer = document.createElement('div');
        modalContainer.innerHTML = modalHtml;
        document.body.appendChild(modalContainer);
        
        const modal = new bootstrap.Modal(document.getElementById('systemStatusModal'));
        modal.show();
        
        // Remover modal do DOM quando fechado
        document.getElementById('systemStatusModal').addEventListener('hidden.bs.modal', () => {
            document.body.removeChild(modalContainer);
        });
        
        // Atualizar estatísticas
        loadAdminStats();
        
        addLog('Verificação de status do sistema concluída', 'success');
    } catch (error) {
        hideLoading();
        addLog(`Erro ao verificar status do sistema: ${error.message}`, 'error');
        showToast('Erro ao verificar status do sistema', 'danger');
    }
}

/**
 * Enviar uma mensagem de teste para uma instância específica
 */
async function sendTestMessage() {
    if (!authState?.isAdmin) return;
    
    const instanceSelect = document.getElementById('adminTestInstance');
    const messageInput = document.getElementById('adminTestMessage');
    const messageTypeSelect = document.getElementById('adminTestMessageType');
    
    if (!instanceSelect || !messageInput || !messageTypeSelect) return;
    
    const instanceId = instanceSelect.value;
    const message = messageInput.value.trim();
    const messageType = messageTypeSelect.value;
    
    if (!instanceId) {
        showToast('Selecione uma instância', 'warning');
        return;
    }
    
    if (!message) {
        showToast('Digite uma mensagem', 'warning');
        return;
    }
    
    try {
        showLoading('Enviando mensagem de teste...');
        
        // Obter opção selecionada
        const selectedOption = instanceSelect.options[instanceSelect.selectedIndex];
        const phone = selectedOption.dataset.phone || '';
        
        // Se não houver telefone, usar o mesmo número da instância
        const recipient = phone || instanceId;
        
        // Definir temporariamente a instância atual para esta requisição
        window.currentInstanceForRequest = instanceId;
        
        // Construir endpoint e payload baseados no tipo de mensagem
        let endpoint = '';
        let payload = {};
        
        switch (messageType) {
            case 'text':
                endpoint = '/chat/send/text';
                payload = {
                    Phone: recipient,
                    Body: message
                };
                break;
                
            case 'image':
                endpoint = '/chat/send/image';
                payload = {
                    Phone: recipient,
                    Image: 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAQAAAAEAAQMAAABmvDolAAAAA1BMVEUAAACnej3aAAAAAXRSTlMAQObYZgAAAB9JREFUaN7twQENAAAAwqD3T20PBxQAAAAAAAAAAHwYCcQAAVstBqEAAAAASUVORK5CYII=',
                    Caption: message
                };
                break;
                
            case 'document':
                endpoint = '/chat/send/document';
                // Criar um documento de texto com a mensagem
                const encoder = new TextEncoder();
                const bytes = encoder.encode(message);
                const base64 = btoa(String.fromCharCode(...new Uint8Array(bytes.buffer)));
                
                payload = {
                    Phone: recipient,
                    Document: `data:application/octet-stream;base64,${base64}`,
                    FileName: 'mensagem-teste.txt'
                };
                break;
                
            case 'location':
                endpoint = '/chat/send/location';
                payload = {
                    Phone: recipient,
                    Latitude: -23.550520,
                    Longitude: -46.633308,
                    Name: message
                };
                break;
        }
        
        // Enviar mensagem
        const response = await makeApiCall(endpoint, 'POST', payload, instanceId);
        
        // Limpar instância temporária
        delete window.currentInstanceForRequest;
        
        hideLoading();
        
        if (response.success) {
            addLog(`Mensagem de teste enviada para instância ${instanceId}`, 'success');
            showToast('Mensagem de teste enviada com sucesso', 'success');
            
            // Limpar campo de mensagem
            messageInput.value = '';
        } else {
            addLog(`Falha ao enviar mensagem para instância ${instanceId}: ${response.error}`, 'error');
            showToast(`Falha ao enviar mensagem: ${response.error}`, 'danger');
        }
    } catch (error) {
        hideLoading();
        delete window.currentInstanceForRequest;
        
        addLog(`Erro ao enviar mensagem de teste: ${error.message}`, 'error');
        showToast('Erro ao enviar mensagem de teste', 'danger');
    }
}

// Exportar funções para uso global
window.initAdminPanel = initAdminPanel;
window.loadAdminStats = loadAdminStats;
window.updateAdminInstancesList = updateAdminInstancesList;
window.checkSystemStatus = checkSystemStatus;
window.sendTestMessage = sendTestMessage; 