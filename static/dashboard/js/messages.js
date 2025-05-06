// Messages Module

/**
 * Alterna a visibilidade dos campos com base no tipo de mensagem selecionado
 */
function showMessageFields() {
    const messageType = document.getElementById('messageType');
    if (!messageType) return;
    
    const type = messageType.value;
    
    // Referências para os diferentes campos
    const textMessageFields = document.getElementById('textMessageFields');
    const mediaMessageFields = document.getElementById('mediaMessageFields');
    const locationMessageFields = document.getElementById('locationMessageFields');
    const contactMessageFields = document.getElementById('contactMessageFields');
    const buttonsMessageFields = document.getElementById('buttonsMessageFields');
    const listMessageFields = document.getElementById('listMessageFields');
    
    // Esconder todos os campos
    if (textMessageFields) textMessageFields.style.display = 'none';
    if (mediaMessageFields) mediaMessageFields.style.display = 'none';
    if (locationMessageFields) locationMessageFields.style.display = 'none';
    if (contactMessageFields) contactMessageFields.style.display = 'none';
    if (buttonsMessageFields) buttonsMessageFields.style.display = 'none';
    if (listMessageFields) listMessageFields.style.display = 'none';
    
    // Mostrar apenas o campo relevante
    switch (type) {
        case 'text':
            if (textMessageFields) textMessageFields.style.display = 'block';
            break;
        case 'image':
        case 'document':
        case 'audio':
        case 'video':
            if (mediaMessageFields) {
                mediaMessageFields.style.display = 'block';
                // Para áudio, não mostrar legenda
                const captionField = document.getElementById('captionField');
                if (captionField) {
                    captionField.style.display = type === 'audio' ? 'none' : 'block';
                }
            }
            break;
        case 'location':
            if (locationMessageFields) locationMessageFields.style.display = 'block';
            break;
        case 'contact':
            if (contactMessageFields) contactMessageFields.style.display = 'block';
            break;
        case 'buttons':
            if (buttonsMessageFields) buttonsMessageFields.style.display = 'block';
            break;
        case 'list':
            if (listMessageFields) listMessageFields.style.display = 'block';
            break;
    }
}

/**
 * Envia uma mensagem com base no tipo selecionado
 * @returns {Promise<boolean>} Sucesso da operação
 */
async function sendMessage() {
    if (!instancesState?.currentInstance) {
        showToast('Selecione uma instância primeiro', 'warning');
        return false;
    }
    
    const messageType = document.getElementById('messageType');
    const messagePhone = document.getElementById('messagePhone');
    
    if (!messageType || !messagePhone) return false;
    
    const phone = messagePhone.value.trim();
    if (!phone) {
        showToast('Informe o número de telefone', 'warning');
        return false;
    }
    
    const type = messageType.value;
    
    try {
        showLoading('Enviando mensagem...');
        addLog(`Enviando mensagem ${type} para ${phone}...`);
        
        let endpoint = '';
        let payload = {};
        
        switch (type) {
            case 'text':
                const messageText = document.getElementById('messageText');
                if (!messageText) return false;
                
                const text = messageText.value.trim();
                if (!text) {
                    hideLoading();
                    showToast('Digite uma mensagem de texto', 'warning');
                    return false;
                }
                
                endpoint = '/chat/send/text';
                payload = {
                    Phone: phone,
                    Body: text
                };
                break;
                
            case 'image':
            case 'document':
            case 'audio':
            case 'video':
                const mediaFile = document.getElementById('mediaFile')?.files[0];
                if (!mediaFile) {
                    hideLoading();
                    showToast('Selecione um arquivo', 'warning');
                    return false;
                }
                
                // Converter para base64
                const base64 = await fileToBase64(mediaFile);
                const caption = document.getElementById('mediaCaption')?.value.trim() || '';
                
                endpoint = `/chat/send/${type}`;
                payload = {
                    Phone: phone,
                    file: base64,
                    filename: mediaFile.name
                };
                
                if (caption && type !== 'audio') {
                    payload.caption = caption;
                }
                break;
                
            case 'location':
                const latitude = document.getElementById('locationLatitude')?.value.trim();
                const longitude = document.getElementById('locationLongitude')?.value.trim();
                const name = document.getElementById('locationName')?.value.trim() || '';
                
                if (!latitude || !longitude) {
                    hideLoading();
                    showToast('Informe latitude e longitude', 'warning');
                    return false;
                }
                
                endpoint = '/chat/send/location';
                payload = {
                    to: phone,
                    latitude: parseFloat(latitude),
                    longitude: parseFloat(longitude)
                };
                
                if (name) {
                    payload.name = name;
                }
                break;
                
            case 'contact':
                const contactName = document.getElementById('contactName')?.value.trim();
                const vcard = document.getElementById('contactVcard')?.value.trim();
                
                if (!contactName || !vcard) {
                    hideLoading();
                    showToast('Informe nome e vCard', 'warning');
                    return false;
                }
                
                endpoint = '/chat/send/contact';
                payload = {
                    to: phone,
                    name: contactName,
                    vcard: vcard
                };
                break;
                
            case 'buttons':
                const buttonsTitle = document.getElementById('buttonsTitle')?.value.trim();
                const buttonsFooter = document.getElementById('buttonsFooter')?.value.trim() || '';
                
                if (!buttonsTitle) {
                    hideLoading();
                    showToast('Informe o título', 'warning');
                    return false;
                }
                
                const buttons = [];
                document.querySelectorAll('.button-text').forEach(button => {
                    const text = button.value.trim();
                    if (text) {
                        buttons.push({ text });
                    }
                });
                
                if (buttons.length === 0) {
                    hideLoading();
                    showToast('Adicione pelo menos um botão', 'warning');
                    return false;
                }
                
                endpoint = '/chat/send/buttons';
                payload = {
                    to: phone,
                    title: buttonsTitle,
                    buttons: buttons
                };
                
                if (buttonsFooter) {
                    payload.footer = buttonsFooter;
                }
                break;
                
            case 'list':
                const listTitle = document.getElementById('listTitle')?.value.trim();
                const listDescription = document.getElementById('listDescription')?.value.trim();
                const listButtonText = document.getElementById('listButtonText')?.value.trim();
                const listFooter = document.getElementById('listFooter')?.value.trim() || '';
                
                if (!listTitle || !listDescription || !listButtonText) {
                    hideLoading();
                    showToast('Preencha os campos obrigatórios', 'warning');
                    return false;
                }
                
                const sections = [];
                document.querySelectorAll('.list-section').forEach(section => {
                    const title = section.querySelector('.section-title')?.value.trim();
                    const items = [];
                    
                    section.querySelectorAll('.section-item').forEach(item => {
                        const itemTitle = item.querySelector('.item-title')?.value.trim();
                        const itemDescription = item.querySelector('.item-description')?.value.trim() || '';
                        
                        if (itemTitle) {
                            items.push({
                                title: itemTitle,
                                description: itemDescription
                            });
                        }
                    });
                    
                    if (title && items.length > 0) {
                        sections.push({
                            title: title,
                            items: items
                        });
                    }
                });
                
                if (sections.length === 0) {
                    hideLoading();
                    showToast('Adicione pelo menos uma seção com itens', 'warning');
                    return false;
                }
                
                endpoint = '/chat/send/list';
                payload = {
                    to: phone,
                    title: listTitle,
                    description: listDescription,
                    buttonText: listButtonText,
                    sections: sections
                };
                
                if (listFooter) {
                    payload.footer = listFooter;
                }
                break;
        }
        
        const response = await makeApiCall(endpoint, 'POST', payload);
        
        hideLoading();
        
        const messageSentAlert = document.getElementById('messageSentAlert');
        const messageErrorAlert = document.getElementById('messageErrorAlert');
        
        if (response.success) {
            // Extrair ID da mensagem da resposta, considerando os diferentes formatos
            const messageId = response.data?.Id || response.data?.id || 'N/A';
            addLog(`Mensagem enviada com sucesso. ID: ${messageId}`, 'success');
            showToast('Mensagem enviada com sucesso');
            
            // Limpar campos conforme o tipo
            if (type === 'text') {
                const messageText = document.getElementById('messageText');
                if (messageText) messageText.value = '';
            } else if (['image', 'document', 'audio', 'video'].includes(type)) {
                const mediaFile = document.getElementById('mediaFile');
                const mediaCaption = document.getElementById('mediaCaption');
                
                if (mediaFile) mediaFile.value = '';
                if (mediaCaption) mediaCaption.value = '';
            }
            
            if (messageSentAlert) {
                messageSentAlert.classList.remove('d-none');
                setTimeout(() => messageSentAlert.classList.add('d-none'), 5000);
            }
            
            return true;
        } else {
            const errorMsg = response.error || 'Erro desconhecido';
            addLog(`Falha ao enviar mensagem: ${errorMsg}`, 'error');
            showToast(`Falha ao enviar mensagem: ${errorMsg}`, 'danger');
            
            if (messageErrorAlert) {
                messageErrorAlert.textContent = errorMsg || 'Erro ao enviar mensagem.';
                messageErrorAlert.classList.remove('d-none');
                setTimeout(() => messageErrorAlert.classList.add('d-none'), 5000);
            }
            
            return false;
        }
    } catch (error) {
        hideLoading();
        addLog('Erro ao tentar enviar mensagem', 'error');
        showToast('Erro ao enviar mensagem', 'danger');
        
        const messageErrorAlert = document.getElementById('messageErrorAlert');
        if (messageErrorAlert) {
            messageErrorAlert.textContent = 'Erro ao conectar com o servidor.';
            messageErrorAlert.classList.remove('d-none');
            setTimeout(() => messageErrorAlert.classList.add('d-none'), 5000);
        }
        
        return false;
    }
}

/**
 * Adiciona um novo botão ao formulário de mensagem com botões
 */
function addButton() {
    const buttonsList = document.getElementById('buttonsList');
    if (!buttonsList) return;
    
    const buttonContainer = document.createElement('div');
    buttonContainer.className = 'input-group mb-2';
    
    buttonContainer.innerHTML = `
        <input type="text" class="form-control button-text" placeholder="Texto do botão">
        <button class="btn btn-outline-danger remove-button" type="button">
            <i class="bi bi-trash"></i>
        </button>
    `;
    
    buttonsList.appendChild(buttonContainer);
    
    // Adicionar evento para remover o botão
    buttonContainer.querySelector('.remove-button').addEventListener('click', removeButton);
}

/**
 * Remove um botão do formulário de mensagem com botões
 * @param {Event} event - Evento de clique
 */
function removeButton(event) {
    const button = event.currentTarget;
    const buttonContainer = button.parentElement;
    
    const buttonsList = document.getElementById('buttonsList');
    if (!buttonsList) return;
    
    // Não remover se for o único botão
    if (buttonsList.children.length <= 1) {
        return;
    }
    
    buttonContainer.remove();
}

/**
 * Adiciona uma nova seção ao formulário de mensagem com lista
 */
function addSection() {
    const listSections = document.getElementById('listSections');
    if (!listSections) return;
    
    const sectionId = Date.now(); // ID único para a seção
    
    const sectionContainer = document.createElement('div');
    sectionContainer.className = 'list-section card mb-3';
    
    sectionContainer.innerHTML = `
        <div class="card-header d-flex justify-content-between align-items-center">
            <div class="fw-bold">Seção</div>
            <button type="button" class="btn btn-sm btn-outline-danger remove-section">
                <i class="bi bi-trash"></i>
            </button>
        </div>
        <div class="card-body">
            <div class="mb-3">
                <label class="form-label">Título da Seção</label>
                <input type="text" class="form-control section-title" placeholder="Título da seção">
            </div>
            <div class="mb-3">
                <label class="form-label">Itens</label>
                <div class="section-items">
                    <!-- Itens serão adicionados aqui -->
                </div>
                <button type="button" class="btn btn-sm btn-outline-secondary add-item" data-section-id="${sectionId}">
                    <i class="bi bi-plus-circle"></i>Adicionar Item
                </button>
            </div>
        </div>
    `;
    
    listSections.appendChild(sectionContainer);
    
    // Adicionar evento para remover a seção
    sectionContainer.querySelector('.remove-section').addEventListener('click', () => {
        const sections = document.querySelectorAll('.list-section');
        
        // Não remover se for a única seção
        if (sections.length <= 1) {
            return;
        }
        
        sectionContainer.remove();
    });
    
    // Adicionar evento para adicionar item
    sectionContainer.querySelector('.add-item').addEventListener('click', (event) => {
        const sectionItems = event.currentTarget.parentElement.querySelector('.section-items');
        addSectionItem(sectionItems);
    });
    
    // Adicionar um item inicial
    const sectionItems = sectionContainer.querySelector('.section-items');
    addSectionItem(sectionItems);
}

/**
 * Adiciona um novo item a uma seção de lista
 * @param {HTMLElement} sectionItems - Container de itens da seção
 */
function addSectionItem(sectionItems) {
    if (!sectionItems) return;
    
    const itemContainer = document.createElement('div');
    itemContainer.className = 'section-item card mb-2';
    
    itemContainer.innerHTML = `
        <div class="card-body">
            <div class="d-flex justify-content-between align-items-center mb-2">
                <div class="fw-bold small">Item</div>
                <button type="button" class="btn btn-sm btn-outline-danger remove-item">
                    <i class="bi bi-trash"></i>
                </button>
            </div>
            <div class="mb-2">
                <input type="text" class="form-control form-control-sm item-title" placeholder="Título do item">
            </div>
            <div>
                <input type="text" class="form-control form-control-sm item-description" placeholder="Descrição (opcional)">
            </div>
        </div>
    `;
    
    sectionItems.appendChild(itemContainer);
    
    // Adicionar evento para remover o item
    itemContainer.querySelector('.remove-item').addEventListener('click', () => {
        const items = sectionItems.querySelectorAll('.section-item');
        
        // Não remover se for o único item
        if (items.length <= 1) {
            return;
        }
        
        itemContainer.remove();
    });
}

/**
 * Inicializa formulários para botões e listas
 */
function initMessageForms() {
    // Inicializar formulário de botões
    const addButtonBtn = document.getElementById('addButtonBtn');
    const buttonsList = document.getElementById('buttonsList');
    
    if (addButtonBtn && buttonsList) {
        // Adicionar botão inicial se não tiver nenhum
        if (buttonsList.children.length === 0) {
            addButton();
        }
        
        // Evento para adicionar novo botão
        addButtonBtn.addEventListener('click', addButton);
        
        // Adicionar eventos para remover botões existentes
        document.querySelectorAll('.remove-button').forEach(btn => {
            btn.addEventListener('click', removeButton);
        });
    }
    
    // Inicializar formulário de listas
    const addSectionBtn = document.getElementById('addSectionBtn');
    const listSections = document.getElementById('listSections');
    
    if (addSectionBtn && listSections) {
        // Adicionar seção inicial se não tiver nenhuma
        if (listSections.children.length === 0) {
            addSection();
        }
        
        // Evento para adicionar nova seção
        addSectionBtn.addEventListener('click', addSection);
    }
}

// Inicialização
document.addEventListener('DOMContentLoaded', () => {
    // Inicializar formulários
    initMessageForms();
    
    // Mostrar campos apropriados com base no tipo inicial
    showMessageFields();
    
    // Registrar evento para mudança no tipo de mensagem
    const messageType = document.getElementById('messageType');
    if (messageType) {
        messageType.addEventListener('change', showMessageFields);
    }
    
    // Registrar evento para o botão de enviar mensagem
    const sendMessageBtn = document.getElementById('sendMessageBtn');
    if (sendMessageBtn) {
        sendMessageBtn.addEventListener('click', sendMessage);
    }
}); 