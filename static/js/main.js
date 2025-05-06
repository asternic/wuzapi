document.addEventListener('DOMContentLoaded', function() {
    // Header scroll effect
    const header = document.querySelector('header');
    const scrollDownBtn = document.querySelector('.scroll-down');
    const menuToggle = document.querySelector('.menu-toggle');
    const mainMenu = document.querySelector('#main-menu');
    
    // Menu mobile toggle
    if (menuToggle && mainMenu) {
        menuToggle.addEventListener('click', function() {
            menuToggle.classList.toggle('active');
            mainMenu.classList.toggle('active');
            document.body.classList.toggle('menu-open');
        });
        
        // Close menu when clicking on a link
        const menuLinks = mainMenu.querySelectorAll('a');
        menuLinks.forEach(link => {
            link.addEventListener('click', function() {
                menuToggle.classList.remove('active');
                mainMenu.classList.remove('active');
                document.body.classList.remove('menu-open');
            });
        });
    }
    
    if (scrollDownBtn) {
        scrollDownBtn.addEventListener('click', function() {
            const featuresSection = document.querySelector('#features');
            if (featuresSection) {
                featuresSection.scrollIntoView({ behavior: 'smooth' });
            }
        });
    }
    
    window.addEventListener('scroll', function() {
        if (window.scrollY > 100) {
            header.classList.add('scrolled');
        } else {
            header.classList.remove('scrolled');
        }
        
        // Animation on scroll
        const animatedElements = document.querySelectorAll('.fade-in, .slide-in-left, .slide-in-right, .zoom-in');
        
        animatedElements.forEach(element => {
            const elementPosition = element.getBoundingClientRect().top;
            const windowHeight = window.innerHeight;
            
            if (elementPosition < windowHeight - 100) {
                element.classList.add('active');
            }
        });
    });
    
    // Dark mode toggle
    const themeToggle = document.querySelector('.theme-toggle');
    
    if (themeToggle) {
        themeToggle.addEventListener('click', function() {
            document.body.classList.toggle('dark-mode');
            
            // Update the icon
            const icon = themeToggle.querySelector('i');
            if (document.body.classList.contains('dark-mode')) {
                icon.classList.remove('fa-moon');
                icon.classList.add('fa-sun');
                localStorage.setItem('theme', 'dark');
            } else {
                icon.classList.remove('fa-sun');
                icon.classList.add('fa-moon');
                localStorage.setItem('theme', 'light');
            }
        });
        
        // Check for saved theme preference
        const savedTheme = localStorage.getItem('theme');
        if (savedTheme === 'dark') {
            document.body.classList.add('dark-mode');
            const icon = themeToggle.querySelector('i');
            icon.classList.remove('fa-moon');
            icon.classList.add('fa-sun');
        }
    }
    
    // Interactive demo
    initDemo();
    
    // Start coffee animation
    animateCoffee();
});

function initDemo() {
    const demoSection = document.querySelector('.demo-section');
    if (!demoSection) return;
    
    const chatBody = document.querySelector('.chat-body');
    const demoButtons = document.querySelectorAll('.demo-btn');
    
    const demoScenarios = {
        'message': [
            { type: 'received', content: 'Olá! Como posso ajudar?' },
            { type: 'sent', content: 'Quero enviar uma mensagem para 55912345678' },
            { type: 'received', content: 'Vou mostrar como usar a API para enviar uma mensagem de texto:' },
            { type: 'code', content: `POST /chat/send/text
{
  "Phone": "55912345678",
  "Body": "Olá, esta é uma mensagem enviada via WUZAPI!"
}`},
            { type: 'received', content: 'Pronto! A mensagem foi enviada com sucesso.' }
        ],
        'media': [
            { type: 'sent', content: 'Como envio uma imagem?' },
            { type: 'typing', content: '' },
            { type: 'received', content: 'Para enviar uma imagem, use o endpoint /chat/send/image:' },
            { type: 'code', content: `POST /chat/send/image
{
  "Phone": "55912345678",
  "Image": "data:image/jpeg;base64,...",
  "Caption": "Veja esta imagem!"
}`},
            { type: 'received', content: 'Você também pode enviar outros tipos de mídia como áudio, documentos e vídeos.' }
        ],
        'webhook': [
            { type: 'sent', content: 'Como configuro webhooks para receber mensagens?' },
            { type: 'typing', content: '' },
            { type: 'received', content: 'Você pode configurar webhooks usando o endpoint /webhook:' },
            { type: 'code', content: `POST /webhook
{
  "webhook": "https://meuservidor.com/webhook",
  "events": ["Message", "ReadReceipt"]
}`},
            { type: 'received', content: 'Isso irá configurar seu servidor para receber notificações de novas mensagens e confirmações de leitura.' }
        ]
    };
    
    if (demoButtons.length > 0 && chatBody) {
        demoButtons.forEach(button => {
            button.addEventListener('click', function() {
                // Add active class to current button and remove from others
                demoButtons.forEach(btn => btn.classList.remove('active-btn'));
                button.classList.add('active-btn');
                
                const scenario = button.getAttribute('data-scenario');
                if (demoScenarios[scenario]) {
                    playDemoScenario(demoScenarios[scenario], chatBody);
                }
            });
        });
        
        // Auto play the first scenario
        setTimeout(() => {
            demoButtons[0].classList.add('active-btn');
            playDemoScenario(demoScenarios['message'], chatBody);
        }, 1000);
    }
}

function playDemoScenario(scenario, chatBody) {
    // Clear the chat
    chatBody.innerHTML = '';
    
    // Play the scenario with delays
    let delay = 0;
    
    scenario.forEach((message, index) => {
        delay += message.type === 'typing' ? 500 : 1000;
        
        setTimeout(() => {
            if (message.type === 'typing') {
                addTypingIndicator(chatBody);
            } else {
                // Remove typing indicator if exists
                const typingIndicator = chatBody.querySelector('.typing-indicator');
                if (typingIndicator) {
                    typingIndicator.remove();
                }
                
                // Add the message
                const messageDiv = document.createElement('div');
                
                if (message.type === 'code') {
                    messageDiv.className = 'chat-message received';
                    
                    // Improved code presentation with better formatting
                    const codeContent = message.content
                        .replace(/\{/g, '{ ')
                        .replace(/\}/g, ' }')
                        .replace(/,/g, ', ');
                    
                    messageDiv.innerHTML = `<pre class="code-snippet">${codeContent}</pre>`;
                } else {
                    messageDiv.className = `chat-message ${message.type}`;
                    messageDiv.textContent = message.content;
                }
                
                chatBody.appendChild(messageDiv);
                chatBody.scrollTop = chatBody.scrollHeight;
            }
        }, delay);
    });
}

function addTypingIndicator(chatBody) {
    // Remove existing typing indicator
    const existingIndicator = chatBody.querySelector('.typing-indicator');
    if (existingIndicator) {
        existingIndicator.remove();
    }
    
    // Add typing indicator
    const typingIndicator = document.createElement('div');
    typingIndicator.className = 'typing-indicator';
    
    for (let i = 0; i < 3; i++) {
        const dot = document.createElement('div');
        dot.className = 'typing-dot';
        typingIndicator.appendChild(dot);
    }
    
    chatBody.appendChild(typingIndicator);
    chatBody.scrollTop = chatBody.scrollHeight;
}

// Coffee animation
function animateCoffee() {
    const coffeeSteam = document.querySelector('.coffee-steam');
    if (coffeeSteam) {
        coffeeSteam.style.opacity = '1';
        
        setTimeout(() => {
            coffeeSteam.style.opacity = '0';
            
            setTimeout(animateCoffee, 3000);
        }, 2000);
    }
}

// Smooth scrolling for all internal links
document.querySelectorAll('a[href^="#"]').forEach(anchor => {
    anchor.addEventListener('click', function(e) {
        e.preventDefault();
        
        const targetId = this.getAttribute('href');
        const targetElement = document.querySelector(targetId);
        
        if (targetElement) {
            // Offset for fixed header
            const headerHeight = document.querySelector('header').offsetHeight;
            const targetPosition = targetElement.getBoundingClientRect().top + window.pageYOffset - headerHeight;
            
            window.scrollTo({
                top: targetPosition,
                behavior: 'smooth'
            });
        }
    });
}); 