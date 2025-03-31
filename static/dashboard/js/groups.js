// Groups Management Module

/**
 * Lista todos os grupos da instância atual
 * @returns {Promise<Array|null>} Lista de grupos ou null
 */
async function listGroups() {
    if (!instancesState?.currentInstance) {
        showToast('Selecione uma instância primeiro', 'warning');
        return null;
    }
    
    try {
        showLoading('Obtendo lista de grupos...');
        addLog('Obtendo grupos...');
        
        const response = await makeApiCall(`/groups/list`);
        
        hideLoading();
        
        const groupsContainer = document.getElementById('groupsContainer');
        if (!groupsContainer) return null;
        
        if (response.success && Array.isArray(response.data)) {
            if (response.data.length === 0) {
                groupsContainer.innerHTML = `
                    <div class="alert alert-info">
                        <i class="bi bi-info-circle"></i> Nenhum grupo encontrado.
                    </div>
                `;
                return [];
            }
            
            groupsContainer.innerHTML = `
                <div class="row">
                    <div class="col-12">
                        <p class="mb-2">Grupos encontrados: ${response.data.length}</p>
                    </div>
                </div>
                <div class="row row-cols-1 row-cols-md-2 g-3" id="groupCards">
                </div>
            `;
            
            const groupCards = document.getElementById('groupCards');
            
            response.data.forEach(group => {
                const card = document.createElement('div');
                card.className = 'col';
                
                const participants = group.participants ? group.participants.length : '?';
                
                card.innerHTML = `
                    <div class="card h-100">
                        <div class="card-body">
                            <h5 class="card-title text-truncate">${group.name || 'Grupo sem nome'}</h5>
                            <p class="card-text">
                                <small class="text-muted">ID: ${group.id.split('@')[0]}</small><br>
                                <small class="text-muted">Participantes: ${participants}</small>
                            </p>
                        </div>
                        <div class="card-footer d-flex justify-content-between">
                            <button class="btn btn-sm btn-outline-primary view-group-btn" data-group-id="${group.id}">
                                <i class="bi bi-info-circle"></i> Detalhes
                            </button>
                            <button class="btn btn-sm btn-outline-success send-group-btn" data-group-id="${group.id}" data-group-name="${group.name || 'Grupo'}">
                                <i class="bi bi-chat"></i> Enviar Mensagem
                            </button>
                        </div>
                    </div>
                `;
                
                groupCards.appendChild(card);
            });
            
            // Adicionar evento aos botões
            document.querySelectorAll('.view-group-btn').forEach(btn => {
                btn.addEventListener('click', () => {
                    const groupId = btn.getAttribute('data-group-id');
                    getGroupInfo(groupId);
                });
            });
            
            document.querySelectorAll('.send-group-btn').forEach(btn => {
                btn.addEventListener('click', () => {
                    const groupId = btn.getAttribute('data-group-id');
                    const groupName = btn.getAttribute('data-group-name');
                    
                    // Preencher o campo de telefone com o ID do grupo
                    const messagePhone = document.getElementById('messagePhone');
                    if (messagePhone) {
                        messagePhone.value = groupId.split('@')[0];
                    }
                    
                    // Mudar para a aba de mensagens
                    const messagesTab = document.getElementById('pills-message-tab');
                    if (messagesTab) {
                        messagesTab.click();
                    }
                    
                    showToast(`Preparado para enviar mensagem para "${groupName}"`);
                });
            });
            
            addLog(`${response.data.length} grupos encontrados`, 'success');
            return response.data;
        } else {
            groupsContainer.innerHTML = `
                <div class="alert alert-danger">
                    <i class="bi bi-exclamation-triangle"></i> Erro ao obter grupos: ${response.error || 'Erro desconhecido'}
                </div>
            `;
            
            addLog(`Falha ao listar grupos: ${response.error || 'Erro desconhecido'}`, 'error');
            return null;
        }
    } catch (error) {
        hideLoading();
        
        const groupsContainer = document.getElementById('groupsContainer');
        if (groupsContainer) {
            groupsContainer.innerHTML = `
                <div class="alert alert-danger">
                    <i class="bi bi-exclamation-triangle"></i> Erro ao conectar com o servidor.
                </div>
            `;
        }
        
        addLog('Erro ao obter lista de grupos', 'error');
        return null;
    }
}

/**
 * Obter informações detalhadas de um grupo
 * @param {string} groupId - ID do grupo
 * @returns {Promise<object|null>} Dados do grupo ou null
 */
async function getGroupInfo(groupId) {
    if (!instancesState?.currentInstance || !groupId) return null;
    
    try {
        showLoading('Obtendo informações do grupo...');
        addLog(`Obtendo informações do grupo ${groupId}...`);
        
        const response = await makeApiCall(`/groups/info`, 'POST', {
            groupId: groupId
        });
        
        hideLoading();
        
        const groupInfoContainer = document.getElementById('groupInfoContainer');
        const groupInfoContent = document.getElementById('groupInfoContent');
        
        if (!groupInfoContainer || !groupInfoContent) return null;
        
        if (response.success && response.data) {
            const group = response.data;
            groupInfoContainer.style.display = 'flex';
            
            let participantsHtml = '';
            if (group.participants && group.participants.length > 0) {
                participantsHtml = `
                    <div class="mt-3">
                        <h6>Participantes (${group.participants.length}):</h6>
                        <div class="table-responsive">
                            <table class="table table-sm table-striped">
                                <thead>
                                    <tr>
                                        <th>Número</th>
                                        <th>Nome</th>
                                        <th>Admin</th>
                                        <th>Ações</th>
                                    </tr>
                                </thead>
                                <tbody>
                `;
                
                group.participants.forEach(participant => {
                    const isAdmin = participant.isAdmin ? 
                        '<i class="bi bi-check-circle-fill text-success"></i>' : 
                        '<i class="bi bi-x-circle text-muted"></i>';
                        
                    participantsHtml += `
                        <tr>
                            <td>${participant.id.split('@')[0]}</td>
                            <td>${participant.name || 'Sem nome'}</td>
                            <td>${isAdmin}</td>
                            <td>
                                <div class="btn-group btn-group-sm">
                                    ${!participant.isAdmin ? 
                                        `<button class="btn btn-outline-success btn-sm make-admin-btn" 
                                            data-participant-id="${participant.id}" title="Tornar admin">
                                            <i class="bi bi-shield-plus"></i>
                                        </button>` : 
                                        `<button class="btn btn-outline-warning btn-sm remove-admin-btn" 
                                            data-participant-id="${participant.id}" title="Remover admin">
                                            <i class="bi bi-shield-minus"></i>
                                        </button>`
                                    }
                                    <button class="btn btn-outline-danger btn-sm remove-participant-btn" 
                                        data-participant-id="${participant.id}" title="Remover do grupo">
                                        <i class="bi bi-person-x"></i>
                                    </button>
                                    <button class="btn btn-outline-primary btn-sm send-msg-to-participant-btn" 
                                        data-participant-id="${participant.id.split('@')[0]}" title="Enviar mensagem">
                                        <i class="bi bi-chat-dots"></i>
                                    </button>
                                </div>
                            </td>
                        </tr>
                    `;
                });
                
                participantsHtml += `
                                </tbody>
                            </table>
                        </div>
                        <button class="btn btn-outline-primary mt-2" id="addParticipantsBtn">
                            <i class="bi bi-person-plus"></i> Adicionar Participantes
                        </button>
                    </div>
                `;
            }
            
            groupInfoContent.innerHTML = `
                <h5 class="mb-3">${group.name || 'Grupo sem nome'}</h5>
                <div class="mb-3">
                    <p><strong>ID:</strong> ${group.id}</p>
                    <p><strong>Criado por:</strong> ${group.owner ? group.owner.split('@')[0] : 'Desconhecido'}</p>
                    <p><strong>Criado em:</strong> ${group.creation ? new Date(group.creation).toLocaleString() : 'Desconhecido'}</p>
                    <p><strong>Descrição:</strong> <span id="groupDescriptionText">${group.description || 'Sem descrição'}</span>
                        <button class="btn btn-sm btn-outline-secondary ms-2" id="editGroupDescriptionBtn">
                            <i class="bi bi-pencil"></i>
                        </button>
                    </p>
                </div>
                ${participantsHtml}
            `;
            
            // Armazenar ID do grupo para outras operações
            groupInfoContainer.setAttribute('data-group-id', groupId);
            groupInfoContainer.setAttribute('data-group-name', group.name || 'Grupo');
            
            // Adicionar eventos aos botões de ação de participantes
            document.querySelectorAll('.make-admin-btn').forEach(btn => {
                btn.addEventListener('click', (e) => {
                    e.preventDefault();
                    const participantId = btn.getAttribute('data-participant-id');
                    promoteToAdmin(groupId, participantId);
                });
            });
            
            document.querySelectorAll('.remove-admin-btn').forEach(btn => {
                btn.addEventListener('click', (e) => {
                    e.preventDefault();
                    const participantId = btn.getAttribute('data-participant-id');
                    demoteFromAdmin(groupId, participantId);
                });
            });
            
            document.querySelectorAll('.remove-participant-btn').forEach(btn => {
                btn.addEventListener('click', (e) => {
                    e.preventDefault();
                    const participantId = btn.getAttribute('data-participant-id');
                    removeParticipant(groupId, participantId);
                });
            });
            
            document.querySelectorAll('.send-msg-to-participant-btn').forEach(btn => {
                btn.addEventListener('click', (e) => {
                    e.preventDefault();
                    const phone = btn.getAttribute('data-participant-id');
                    const messagePhone = document.getElementById('messagePhone');
                    if (messagePhone) {
                        messagePhone.value = phone;
                    }
                    const messagesTab = document.getElementById('pills-message-tab');
                    if (messagesTab) {
                        messagesTab.click();
                    }
                    showToast(`Preparado para enviar mensagem para ${phone}`);
                });
            });
            
            // Adicionar evento ao botão de adicionar participantes
            const addParticipantsBtn = document.getElementById('addParticipantsBtn');
            if (addParticipantsBtn) {
                addParticipantsBtn.addEventListener('click', () => {
                    showAddParticipantsModal(groupId);
                });
            }
            
            // Adicionar evento ao botão de editar descrição
            const editGroupDescriptionBtn = document.getElementById('editGroupDescriptionBtn');
            if (editGroupDescriptionBtn) {
                editGroupDescriptionBtn.addEventListener('click', () => {
                    showEditGroupDescriptionModal(groupId, group.description || '');
                });
            }
            
            addLog(`Informações do grupo obtidas com sucesso`, 'success');
            return group;
        } else {
            groupInfoContainer.style.display = 'flex';
            groupInfoContent.innerHTML = `
                <div class="alert alert-danger">
                    <i class="bi bi-exclamation-triangle"></i> Erro ao obter informações do grupo: ${response.error || 'Erro desconhecido'}
                </div>
            `;
            
            addLog(`Falha ao obter informações do grupo: ${response.error || 'Erro desconhecido'}`, 'error');
            return null;
        }
    } catch (error) {
        hideLoading();
        
        const groupInfoContainer = document.getElementById('groupInfoContainer');
        const groupInfoContent = document.getElementById('groupInfoContent');
        
        if (groupInfoContainer && groupInfoContent) {
            groupInfoContainer.style.display = 'flex';
            groupInfoContent.innerHTML = `
                <div class="alert alert-danger">
                    <i class="bi bi-exclamation-triangle"></i> Erro ao conectar com o servidor.
                </div>
            `;
        }
        
        addLog('Erro ao obter informações do grupo', 'error');
        return null;
    }
}

/**
 * Obter link de convite para um grupo
 * @param {string} groupId - ID do grupo
 * @returns {Promise<string|null>} Link de convite ou null
 */
async function getGroupInviteLink() {
    const groupInfoContainer = document.getElementById('groupInfoContainer');
    if (!groupInfoContainer) return null;
    
    const groupId = groupInfoContainer.getAttribute('data-group-id');
    
    if (!instancesState?.currentInstance || !groupId) return null;
    
    try {
        showLoading('Obtendo link de convite...');
        addLog(`Obtendo link de convite para o grupo ${groupId}...`);
        
        const response = await makeApiCall(`/groups/invitelink`, 'POST', {
            groupId: groupId
        });
        
        hideLoading();
        
        if (response.success && response.data && response.data.link) {
            addLog(`Link de convite obtido com sucesso`, 'success');
            
            // Mostrar o link obtido em um modal
            const modalHtml = createModal({
                id: 'inviteLinkModal',
                title: 'Link de Convite',
                body: `
                    <div class="mb-3">
                        <label class="form-label">Link de convite para o grupo:</label>
                        <div class="input-group">
                            <input type="text" class="form-control" id="inviteLinkInput" value="${response.data.link}" readonly>
                            <button class="btn btn-outline-primary" id="copyLinkBtn">
                                <i class="bi bi-clipboard"></i>Copiar
                            </button>
                        </div>
                    </div>
                `,
                footer: `
                    <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">Fechar</button>
                    <button type="button" class="btn btn-primary" id="resetInviteLinkBtn">
                        <i class="bi bi-arrow-repeat"></i>Gerar Novo Link
                    </button>
                `,
                onShow: (modal) => {
                    // Adicionar evento ao botão de copiar
                    document.getElementById('copyLinkBtn').addEventListener('click', () => {
                        const inviteLinkInput = document.getElementById('inviteLinkInput');
                        inviteLinkInput.select();
                        document.execCommand('copy');
                        showToast('Link copiado para a área de transferência');
                    });
                    
                    // Adicionar evento ao botão de resetar link
                    document.getElementById('resetInviteLinkBtn').addEventListener('click', async () => {
                        try {
                            showLoading('Gerando novo link...');
                            
                            const resetResponse = await makeApiCall(`/groups/resetinvitelink`, 'POST', {
                                groupId: groupId
                            });
                            
                            hideLoading();
                            
                            if (resetResponse.success && resetResponse.data && resetResponse.data.link) {
                                document.getElementById('inviteLinkInput').value = resetResponse.data.link;
                                showToast('Novo link gerado com sucesso');
                            } else {
                                showToast(`Falha ao gerar novo link: ${resetResponse.error || 'Erro desconhecido'}`, 'danger');
                            }
                        } catch (error) {
                            hideLoading();
                            showToast('Erro ao gerar novo link', 'danger');
                        }
                    });
                }
            });
            
            return response.data.link;
        } else {
            showToast(`Falha ao obter link: ${response.error || 'Erro desconhecido'}`, 'danger');
            addLog(`Falha ao obter link de convite: ${response.error || 'Erro desconhecido'}`, 'error');
            return null;
        }
    } catch (error) {
        hideLoading();
        showToast('Erro ao obter link de convite', 'danger');
        addLog('Erro ao obter link de convite', 'error');
        return null;
    }
}

/**
 * Criar um novo grupo
 * @returns {Promise<object|null>} Dados do grupo criado ou null
 */
async function createGroup() {
    if (!instancesState?.currentInstance) {
        showToast('Selecione uma instância primeiro', 'warning');
        return null;
    }
    
    // Abrir modal para criar grupo
    const modalHtml = createModal({
        id: 'createGroupModal',
        title: 'Criar Novo Grupo',
        body: `
            <div class="mb-3">
                <label for="newGroupName" class="form-label">Nome do Grupo</label>
                <input type="text" class="form-control" id="newGroupName" placeholder="Ex: Grupo de Trabalho">
            </div>
            <div class="mb-3">
                <label for="newGroupParticipants" class="form-label">Participantes (um por linha)</label>
                <textarea class="form-control" id="newGroupParticipants" rows="5" placeholder="551199999999&#10;551198888888"></textarea>
                <div class="form-text">Digite os números completos com código do país, sem + ou espaços</div>
            </div>
        `,
        footer: `
            <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">Cancelar</button>
            <button type="button" class="btn btn-primary" id="confirmCreateGroupBtn">
                <i class="bi bi-people"></i>Criar Grupo
            </button>
        `,
        onShow: (modal) => {
            // Adicionar evento ao botão de criar
            document.getElementById('confirmCreateGroupBtn').addEventListener('click', async () => {
                const groupName = document.getElementById('newGroupName').value.trim();
                const participantsText = document.getElementById('newGroupParticipants').value.trim();
                
                if (!groupName) {
                    showToast('Informe o nome do grupo', 'warning');
                    return;
                }
                
                if (!participantsText) {
                    showToast('Adicione pelo menos um participante', 'warning');
                    return;
                }
                
                // Processar participantes
                const participants = participantsText.split('\n')
                    .map(p => p.trim())
                    .filter(p => p.length > 0);
                
                if (participants.length === 0) {
                    showToast('Adicione pelo menos um participante', 'warning');
                    return;
                }
                
                try {
                    showLoading('Criando grupo...');
                    modal.hide();
                    
                    const response = await makeApiCall(`/groups/create`, 'POST', {
                        name: groupName,
                        participants: participants
                    });
                    
                    hideLoading();
                    
                    if (response.success && response.data) {
                        addLog(`Grupo "${groupName}" criado com sucesso`, 'success');
                        showToast(`Grupo "${groupName}" criado com sucesso`);
                        
                        // Atualizar lista de grupos
                        listGroups();
                        return response.data;
                    } else {
                        addLog(`Falha ao criar grupo: ${response.error || 'Erro desconhecido'}`, 'error');
                        showToast(`Falha ao criar grupo: ${response.error || 'Erro desconhecido'}`, 'danger');
                        return null;
                    }
                } catch (error) {
                    hideLoading();
                    addLog('Erro ao criar grupo', 'error');
                    showToast('Erro ao criar grupo', 'danger');
                    return null;
                }
            });
        }
    });
}

/**
 * Promover um participante a administrador
 * @param {string} groupId - ID do grupo
 * @param {string} participantId - ID do participante
 * @returns {Promise<boolean>} Sucesso da operação
 */
async function promoteToAdmin(groupId, participantId) {
    if (!instancesState?.currentInstance || !groupId || !participantId) return false;
    
    try {
        showLoading('Promovendo participante...');
        addLog(`Promovendo ${participantId.split('@')[0]} a administrador...`);
        
        const response = await makeApiCall(`/groups/promote`, 'POST', {
            groupId: groupId,
            participantId: participantId
        });
        
        hideLoading();
        
        if (response.success) {
            addLog('Participante promovido a administrador com sucesso', 'success');
            showToast('Participante promovido a administrador');
            
            // Atualizar informações do grupo
            getGroupInfo(groupId);
            return true;
        } else {
            addLog(`Falha ao promover participante: ${response.error || 'Erro desconhecido'}`, 'error');
            showToast(`Falha ao promover participante: ${response.error || 'Erro desconhecido'}`, 'danger');
            return false;
        }
    } catch (error) {
        hideLoading();
        addLog('Erro ao promover participante', 'error');
        showToast('Erro ao promover participante', 'danger');
        return false;
    }
}

/**
 * Remover status de administrador de um participante
 * @param {string} groupId - ID do grupo
 * @param {string} participantId - ID do participante
 * @returns {Promise<boolean>} Sucesso da operação
 */
async function demoteFromAdmin(groupId, participantId) {
    if (!instancesState?.currentInstance || !groupId || !participantId) return false;
    
    try {
        showLoading('Rebaixando administrador...');
        addLog(`Rebaixando ${participantId.split('@')[0]} de administrador...`);
        
        const response = await makeApiCall(`/groups/demote`, 'POST', {
            groupId: groupId,
            participantId: participantId
        });
        
        hideLoading();
        
        if (response.success) {
            addLog('Administrador rebaixado com sucesso', 'success');
            showToast('Administrador rebaixado com sucesso');
            
            // Atualizar informações do grupo
            getGroupInfo(groupId);
            return true;
        } else {
            addLog(`Falha ao rebaixar administrador: ${response.error || 'Erro desconhecido'}`, 'error');
            showToast(`Falha ao rebaixar administrador: ${response.error || 'Erro desconhecido'}`, 'danger');
            return false;
        }
    } catch (error) {
        hideLoading();
        addLog('Erro ao rebaixar administrador', 'error');
        showToast('Erro ao rebaixar administrador', 'danger');
        return false;
    }
}

/**
 * Remover um participante do grupo
 * @param {string} groupId - ID do grupo
 * @param {string} participantId - ID do participante
 * @returns {Promise<boolean>} Sucesso da operação
 */
async function removeParticipant(groupId, participantId) {
    if (!instancesState?.currentInstance || !groupId || !participantId) return false;
    
    if (!confirm(`Tem certeza que deseja remover ${participantId.split('@')[0]} do grupo?`)) {
        return false;
    }
    
    try {
        showLoading('Removendo participante...');
        addLog(`Removendo ${participantId.split('@')[0]} do grupo...`);
        
        const response = await makeApiCall(`/groups/remove`, 'POST', {
            groupId: groupId,
            participantId: participantId
        });
        
        hideLoading();
        
        if (response.success) {
            addLog('Participante removido com sucesso', 'success');
            showToast('Participante removido com sucesso');
            
            // Atualizar informações do grupo
            getGroupInfo(groupId);
            return true;
        } else {
            addLog(`Falha ao remover participante: ${response.error || 'Erro desconhecido'}`, 'error');
            showToast(`Falha ao remover participante: ${response.error || 'Erro desconhecido'}`, 'danger');
            return false;
        }
    } catch (error) {
        hideLoading();
        addLog('Erro ao remover participante', 'error');
        showToast('Erro ao remover participante', 'danger');
        return false;
    }
}

/**
 * Exibir modal para adicionar participantes a um grupo
 * @param {string} groupId - ID do grupo
 */
function showAddParticipantsModal(groupId) {
    if (!instancesState?.currentInstance || !groupId) return;
    
    // Abrir modal para adicionar participantes
    createModal({
        id: 'addParticipantsModal',
        title: 'Adicionar Participantes',
        body: `
            <div class="mb-3">
                <label for="newParticipants" class="form-label">Participantes (um por linha)</label>
                <textarea class="form-control" id="newParticipants" rows="5" placeholder="551199999999&#10;551198888888"></textarea>
                <div class="form-text">Digite os números completos com código do país, sem + ou espaços</div>
            </div>
        `,
        footer: `
            <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">Cancelar</button>
            <button type="button" class="btn btn-primary" id="confirmAddParticipantsBtn">
                <i class="bi bi-person-plus"></i>Adicionar
            </button>
        `,
        onShow: (modal) => {
            // Adicionar evento ao botão de adicionar
            document.getElementById('confirmAddParticipantsBtn').addEventListener('click', async () => {
                const participantsText = document.getElementById('newParticipants').value.trim();
                
                if (!participantsText) {
                    showToast('Adicione pelo menos um participante', 'warning');
                    return;
                }
                
                // Processar participantes
                const participants = participantsText.split('\n')
                    .map(p => p.trim())
                    .filter(p => p.length > 0);
                
                if (participants.length === 0) {
                    showToast('Adicione pelo menos um participante', 'warning');
                    return;
                }
                
                try {
                    showLoading('Adicionando participantes...');
                    modal.hide();
                    
                    const response = await makeApiCall(`/groups/add`, 'POST', {
                        groupId: groupId,
                        participants: participants
                    });
                    
                    hideLoading();
                    
                    if (response.success) {
                        addLog(`${participants.length} participantes adicionados com sucesso`, 'success');
                        showToast(`${participants.length} participantes adicionados`);
                        
                        // Atualizar informações do grupo
                        getGroupInfo(groupId);
                    } else {
                        addLog(`Falha ao adicionar participantes: ${response.error || 'Erro desconhecido'}`, 'error');
                        showToast(`Falha ao adicionar participantes: ${response.error || 'Erro desconhecido'}`, 'danger');
                    }
                } catch (error) {
                    hideLoading();
                    addLog('Erro ao adicionar participantes', 'error');
                    showToast('Erro ao adicionar participantes', 'danger');
                }
            });
        }
    });
}

/**
 * Exibir modal para editar a descrição de um grupo
 * @param {string} groupId - ID do grupo
 * @param {string} currentDescription - Descrição atual
 */
function showEditGroupDescriptionModal(groupId, currentDescription) {
    if (!instancesState?.currentInstance || !groupId) return;
    
    // Abrir modal para editar descrição
    createModal({
        id: 'editGroupDescriptionModal',
        title: 'Editar Descrição do Grupo',
        body: `
            <div class="mb-3">
                <label for="groupDescription" class="form-label">Descrição</label>
                <textarea class="form-control" id="groupDescription" rows="4">${currentDescription}</textarea>
            </div>
        `,
        footer: `
            <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">Cancelar</button>
            <button type="button" class="btn btn-primary" id="confirmUpdateDescriptionBtn">
                <i class="bi bi-save"></i>Salvar
            </button>
        `,
        onShow: (modal) => {
            // Adicionar evento ao botão de salvar
            document.getElementById('confirmUpdateDescriptionBtn').addEventListener('click', async () => {
                const description = document.getElementById('groupDescription').value.trim();
                
                try {
                    showLoading('Atualizando descrição...');
                    modal.hide();
                    
                    const response = await makeApiCall(`/groups/updatedescription`, 'POST', {
                        groupId: groupId,
                        description: description
                    });
                    
                    hideLoading();
                    
                    if (response.success) {
                        addLog('Descrição atualizada com sucesso', 'success');
                        showToast('Descrição atualizada com sucesso');
                        
                        // Atualizar exibição da descrição
                        const groupDescriptionText = document.getElementById('groupDescriptionText');
                        if (groupDescriptionText) {
                            groupDescriptionText.textContent = description || 'Sem descrição';
                        }
                    } else {
                        addLog(`Falha ao atualizar descrição: ${response.error || 'Erro desconhecido'}`, 'error');
                        showToast(`Falha ao atualizar descrição: ${response.error || 'Erro desconhecido'}`, 'danger');
                    }
                } catch (error) {
                    hideLoading();
                    addLog('Erro ao atualizar descrição', 'error');
                    showToast('Erro ao atualizar descrição', 'danger');
                }
            });
        }
    });
}

/**
 * Editar o nome de um grupo
 * @param {string} groupId - ID do grupo
 * @returns {Promise<boolean>} Sucesso da operação
 */
async function editGroupName(groupId) {
    if (!instancesState?.currentInstance || !groupId) return false;
    
    const groupInfoContainer = document.getElementById('groupInfoContainer');
    if (!groupInfoContainer) return false;
    
    const groupName = groupInfoContainer.getAttribute('data-group-name') || '';
    
    // Abrir modal para editar nome
    createModal({
        id: 'editGroupNameModal',
        title: 'Editar Nome do Grupo',
        body: `
            <div class="mb-3">
                <label for="groupName" class="form-label">Nome</label>
                <input type="text" class="form-control" id="groupName" value="${groupName}">
            </div>
        `,
        footer: `
            <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">Cancelar</button>
            <button type="button" class="btn btn-primary" id="confirmUpdateNameBtn">
                <i class="bi bi-save"></i>Salvar
            </button>
        `,
        onShow: (modal) => {
            // Adicionar evento ao botão de salvar
            document.getElementById('confirmUpdateNameBtn').addEventListener('click', async () => {
                const name = document.getElementById('groupName').value.trim();
                
                if (!name) {
                    showToast('Informe o nome do grupo', 'warning');
                    return;
                }
                
                try {
                    showLoading('Atualizando nome...');
                    modal.hide();
                    
                    const response = await makeApiCall(`/groups/updatename`, 'POST', {
                        groupId: groupId,
                        name: name
                    });
                    
                    hideLoading();
                    
                    if (response.success) {
                        addLog('Nome atualizado com sucesso', 'success');
                        showToast('Nome atualizado com sucesso');
                        
                        // Atualizar informações do grupo
                        getGroupInfo(groupId);
                        return true;
                    } else {
                        addLog(`Falha ao atualizar nome: ${response.error || 'Erro desconhecido'}`, 'error');
                        showToast(`Falha ao atualizar nome: ${response.error || 'Erro desconhecido'}`, 'danger');
                        return false;
                    }
                } catch (error) {
                    hideLoading();
                    addLog('Erro ao atualizar nome', 'error');
                    showToast('Erro ao atualizar nome', 'danger');
                    return false;
                }
            });
        }
    });
}

/**
 * Alterar a foto de um grupo
 * @param {string} groupId - ID do grupo
 * @returns {Promise<boolean>} Sucesso da operação
 */
async function changeGroupPhoto(groupId) {
    if (!instancesState?.currentInstance || !groupId) return false;
    
    // Abrir modal para alterar foto
    createModal({
        id: 'changeGroupPhotoModal',
        title: 'Alterar Foto do Grupo',
        body: `
            <div class="mb-3">
                <label for="groupPhoto" class="form-label">Selecione uma imagem</label>
                <input type="file" class="form-control" id="groupPhoto" accept="image/*">
                <div class="form-text">Tamanho máximo: 5MB. Formatos aceitos: JPG, PNG.</div>
            </div>
        `,
        footer: `
            <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">Cancelar</button>
            <button type="button" class="btn btn-primary" id="confirmUpdatePhotoBtn">
                <i class="bi bi-upload"></i>Enviar
            </button>
        `,
        onShow: (modal) => {
            // Adicionar evento ao botão de enviar
            document.getElementById('confirmUpdatePhotoBtn').addEventListener('click', async () => {
                const photoInput = document.getElementById('groupPhoto');
                
                if (!photoInput?.files || photoInput.files.length === 0) {
                    showToast('Selecione uma imagem', 'warning');
                    return;
                }
                
                const photo = photoInput.files[0];
                
                if (photo.size > 5 * 1024 * 1024) {
                    showToast('A imagem deve ter no máximo 5MB', 'warning');
                    return;
                }
                
                try {
                    showLoading('Enviando foto...');
                    modal.hide();
                    
                    // Converter para base64
                    const base64 = await fileToBase64(photo);
                    
                    const response = await makeApiCall(`/groups/updatephoto`, 'POST', {
                        groupId: groupId,
                        image: base64
                    });
                    
                    hideLoading();
                    
                    if (response.success) {
                        addLog('Foto atualizada com sucesso', 'success');
                        showToast('Foto atualizada com sucesso');
                        return true;
                    } else {
                        addLog(`Falha ao atualizar foto: ${response.error || 'Erro desconhecido'}`, 'error');
                        showToast(`Falha ao atualizar foto: ${response.error || 'Erro desconhecido'}`, 'danger');
                        return false;
                    }
                } catch (error) {
                    hideLoading();
                    addLog('Erro ao atualizar foto', 'error');
                    showToast('Erro ao atualizar foto', 'danger');
                    return false;
                }
            });
        }
    });
}

/**
 * Sair de um grupo
 * @param {string} groupId - ID do grupo
 * @returns {Promise<boolean>} Sucesso da operação
 */
async function leaveGroup(groupId) {
    if (!instancesState?.currentInstance || !groupId) return false;
    
    if (!confirm('Tem certeza que deseja sair deste grupo?')) {
        return false;
    }
    
    try {
        showLoading('Saindo do grupo...');
        
        const response = await makeApiCall(`/groups/leave`, 'POST', {
            groupId: groupId
        });
        
        hideLoading();
        
        if (response.success) {
            addLog('Você saiu do grupo com sucesso', 'success');
            showToast('Você saiu do grupo com sucesso');
            
            // Fechar informações do grupo
            const groupInfoContainer = document.getElementById('groupInfoContainer');
            if (groupInfoContainer) {
                groupInfoContainer.style.display = 'none';
            }
            
            // Atualizar lista de grupos
            listGroups();
            return true;
        } else {
            addLog(`Falha ao sair do grupo: ${response.error || 'Erro desconhecido'}`, 'error');
            showToast(`Falha ao sair do grupo: ${response.error || 'Erro desconhecido'}`, 'danger');
            return false;
        }
    } catch (error) {
        hideLoading();
        addLog('Erro ao sair do grupo', 'error');
        showToast('Erro ao sair do grupo', 'danger');
        return false;
    }
}

/**
 * Cria e exibe um modal dinâmico
 * @param {object} options - Opções do modal
 * @param {string} options.id - ID do modal
 * @param {string} options.title - Título do modal
 * @param {string} options.body - Conteúdo do corpo do modal
 * @param {string} options.footer - Conteúdo do rodapé do modal
 * @param {function} options.onShow - Callback executado ao mostrar o modal
 * @returns {bootstrap.Modal} Instância do modal criada
 */
function createModal(options) {
    const modalHtml = `
        <div class="modal fade" id="${options.id}" tabindex="-1" aria-hidden="true">
            <div class="modal-dialog">
                <div class="modal-content">
                    <div class="modal-header">
                        <h5 class="modal-title">${options.title}</h5>
                        <button type="button" class="btn-close" data-bs-dismiss="modal" aria-label="Close"></button>
                    </div>
                    <div class="modal-body">
                        ${options.body}
                    </div>
                    <div class="modal-footer">
                        ${options.footer}
                    </div>
                </div>
            </div>
        </div>
    `;
    
    // Adicionar modal ao corpo do documento
    const modalContainer = document.createElement('div');
    modalContainer.innerHTML = modalHtml;
    document.body.appendChild(modalContainer);
    
    // Mostrar modal
    const modal = new bootstrap.Modal(document.getElementById(options.id));
    modal.show();
    
    // Executar callback onShow se fornecido
    if (typeof options.onShow === 'function') {
        options.onShow(modal);
    }
    
    // Remover modal do DOM quando fechado
    document.getElementById(options.id).addEventListener('hidden.bs.modal', () => {
        modalContainer.remove();
    });
    
    return modal;
}

// Inicialização
document.addEventListener('DOMContentLoaded', () => {
    // Registrar eventos dos botões de grupo
    const listGroupsBtn = document.getElementById('listGroupsBtn');
    if (listGroupsBtn) {
        listGroupsBtn.addEventListener('click', listGroups);
    }
    
    const createGroupBtn = document.getElementById('createGroupBtn');
    if (createGroupBtn) {
        createGroupBtn.addEventListener('click', createGroup);
    }
    
    const getGroupInviteLinkBtn = document.getElementById('getGroupInviteLinkBtn');
    if (getGroupInviteLinkBtn) {
        getGroupInviteLinkBtn.addEventListener('click', getGroupInviteLink);
    }
    
    const editGroupNameBtn = document.getElementById('editGroupNameBtn');
    if (editGroupNameBtn) {
        editGroupNameBtn.addEventListener('click', () => {
            const groupInfoContainer = document.getElementById('groupInfoContainer');
            if (!groupInfoContainer) return;
            
            const groupId = groupInfoContainer.getAttribute('data-group-id');
            if (groupId) {
                editGroupName(groupId);
            }
        });
    }
    
    const changeGroupPhotoBtn = document.getElementById('changeGroupPhotoBtn');
    if (changeGroupPhotoBtn) {
        changeGroupPhotoBtn.addEventListener('click', () => {
            const groupInfoContainer = document.getElementById('groupInfoContainer');
            if (!groupInfoContainer) return;
            
            const groupId = groupInfoContainer.getAttribute('data-group-id');
            if (groupId) {
                changeGroupPhoto(groupId);
            }
        });
    }
    
    const leaveGroupBtn = document.getElementById('leaveGroupBtn');
    if (leaveGroupBtn) {
        leaveGroupBtn.addEventListener('click', () => {
            const groupInfoContainer = document.getElementById('groupInfoContainer');
            if (!groupInfoContainer) return;
            
            const groupId = groupInfoContainer.getAttribute('data-group-id');
            if (groupId) {
                leaveGroup(groupId);
            }
        });
    }
    
    const closeGroupInfoBtn = document.getElementById('closeGroupInfoBtn');
    if (closeGroupInfoBtn) {
        closeGroupInfoBtn.addEventListener('click', () => {
            const groupInfoContainer = document.getElementById('groupInfoContainer');
            if (groupInfoContainer) {
                groupInfoContainer.style.display = 'none';
            }
        });
    }
    
    // Lidar com o botão para obter informações específicas de um grupo
    const getGroupInfoBtn = document.getElementById('getGroupInfoBtn');
    if (getGroupInfoBtn) {
        getGroupInfoBtn.addEventListener('click', () => {
            const groupJidInput = document.getElementById('groupJidInput');
            if (!groupJidInput) return;
            
            const groupId = groupJidInput.value.trim();
            if (!groupId) {
                showToast('Informe o ID do grupo', 'warning');
                return;
            }
            
            // Verificar se já tem o sufixo @g.us
            let fullGroupId = groupId;
            if (!fullGroupId.includes('@g.us')) {
                fullGroupId += '@g.us';
            }
            
            getGroupInfo(fullGroupId);
        });
    }
}); 