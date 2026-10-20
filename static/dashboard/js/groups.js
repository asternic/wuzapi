// Groups Manager JavaScript
// Manages all WhatsApp groups functionality
// 
// ADMIN PERMISSIONS: Modified to always show admin actions and let backend handle permissions
// This ensures all group management features are visible to users

let groupsCache = {};
let contactsCache = {};
let currentGroupId = null;
let groupsUpdateInterval = null;

// Function to identify group type based on API data
function identificarTipo(data) {
    if (data.IsParent === true) {
        return "COMMUNITY";
    }
    
    if (data.LinkedParentJID !== "" && data.LinkedParentJID !== null) {
        return "COMMUNITY_GROUP";
    }
    
    return "NORMAL_GROUP";
}

// Function to get group type label and icon
function getGroupTypeInfo(groupData) {
    const tipo = identificarTipo(groupData);
    
    switch (tipo) {
        case "COMMUNITY":
            return {
                label: "Community",
                icon: "building",
                color: "purple",
                description: "Main community"
            };
        case "COMMUNITY_GROUP":
            return {
                label: "Community Group",
                icon: "users",
                color: "blue",
                description: "Group within community"
            };
        case "NORMAL_GROUP":
        default:
            return {
                label: "Group",
                icon: "users",
                color: "green",
                description: "Regular group"
            };
    }
}

// API function to get contacts without automatic download
async function getContactsForGroups() {
    const token = getLocalStorageItem('token');
    const myHeaders = new Headers();
    myHeaders.append('token', token);
    myHeaders.append('Content-Type', 'application/json');
    try {
        const res = await fetch(baseUrl + "/user/contacts", {
            method: "GET",
            headers: myHeaders,
        });
        const data = await res.json();
        if (data.code === 200) {
            const transformedContacts = Object.entries(data.data).map(([phone, contact]) => ({
                FullName: contact.FullName || "",
                PushName: contact.PushName || "",
                Phone: phone.split('@')[0] // Remove the @s.whatsapp.net part
            }));
            // Return contacts without downloading - for dashboard use
            return transformedContacts;
        } else {
            throw new Error(`API returned code ${data.code}`);
        }
    } catch (error) {
        console.error("Error fetching contacts:", error);
        throw error;
    }
}

// Initialize groups functionality when DOM is ready
document.addEventListener('DOMContentLoaded', function() {
    initializeGroupsEvents();
    initializeGroupsModals();
});

function initializeGroupsEvents() {
    // Main dashboard group cards
    $('#groupsList').on('click', function() {
        showGroupsManager();
    });

    $('#createGroup').on('click', function() {
        showCreateGroupModal();
    });

    $('#joinGroup').on('click', function() {
        showJoinGroupModal();
    });

    // Groups manager events
    $('#backToDashboard').on('click', function() {
        hideGroupsManager();
    });

    $('#createNewGroup, #createFirstGroup').on('click', function() {
        showCreateGroupModal();
    });

    // Refresh groups
    $('#refreshGroups').on('click', function() {
        loadGroups();
    });

    // Search and filter
    $('#groupsSearch').on('input', function() {
        filterGroups();
    });

    $('#groupsFilter').on('change', function() {
        filterGroups();
    });
}

function initializeGroupsModals() {
    // Initialize dropdowns
    $('#groupsFilter').dropdown();
    $('#participantsDropdown').dropdown({
        allowAdditions: true,
        hideAdditions: false
    });
    $('#addParticipantsDropdown').dropdown({
        allowAdditions: true,
        hideAdditions: false
    });
    $('#disappearingTimerSelect').dropdown();

    // Load contacts button event
    $('#loadContactsBtn').on('click', async function() {
        const button = $(this);
        const statusDiv = $('#contactsStatus');
        const statusText = $('#contactsStatusText');
        
        // Show loading state
        button.addClass('loading').prop('disabled', true);
        statusDiv.show().removeClass('success error').addClass('info');
        statusText.text('Loading contacts...');
        
        try {
            await loadContacts();
            populateParticipantsDropdown();
            
            // Show success state
            statusDiv.removeClass('info').addClass('success');
            statusText.html('<i class="check icon"></i> Contacts loaded successfully! You can now select participants.');
            
            // Hide status after 3 seconds
            setTimeout(() => {
                statusDiv.fadeOut();
            }, 3000);
            
        } catch (error) {
            console.error('Error loading contacts:', error);
            statusDiv.removeClass('info').addClass('error');
            statusText.html('<i class="exclamation triangle icon"></i> Failed to load contacts. Please try again.');
        } finally {
            button.removeClass('loading').prop('disabled', false);
        }
    });

    // Load contacts for participants management
    $('#loadContactsForParticipantsBtn').on('click', async function() {
        const button = $(this);
        const statusDiv = $('#participantsContactsStatus');
        const statusText = $('#participantsContactsStatusText');
        
        button.addClass('loading').prop('disabled', true);
        statusDiv.show().removeClass('success error').addClass('info');
        statusText.text('Loading contacts...');
        
        try {
            await loadContacts();
            populateAddParticipantsDropdown();
            
            statusDiv.removeClass('info').addClass('success');
            statusText.html('<i class="check icon"></i> Contacts loaded successfully!');
            
            setTimeout(() => {
                statusDiv.fadeOut();
            }, 3000);
            
        } catch (error) {
            console.error('Error loading contacts:', error);
            statusDiv.removeClass('info').addClass('error');
            statusText.html('<i class="exclamation triangle icon"></i> Failed to load contacts. Please try again.');
        } finally {
            button.removeClass('loading').prop('disabled', false);
        }
    });

    // File input events for group photo
    $('#changeGroupPhotoBtn').on('click', function() {
        $('#groupPhotoInput').click();
    });

    $('#groupPhotoInput').on('change', function() {
        if (this.files && this.files[0]) {
            // Handle photo upload
            uploadGroupPhoto(this.files[0]);
        }
    });

    // Form submissions
    $('#createGroupSubmit').on('click', function() {
        createGroup();
    });

    $('#joinGroupSubmit').on('click', function() {
        joinGroup();
    });

    $('#previewGroupBtn').on('click', function() {
        previewGroup();
    });

    $('#saveGroupInfoBtn').on('click', function() {
        saveGroupInfo();
    });

    $('#saveGroupSettingsBtn').on('click', function() {
        saveGroupSettings();
    });

    // Group action buttons
    $('#editGroupInfoBtn').on('click', function() {
        showEditGroupInfoModal();
    });

    $('#manageParticipantsBtn').on('click', function() {
        showManageParticipantsModal();
    });

    $('#groupSettingsBtn').on('click', function() {
        showGroupSettingsModal();
    });

    $('#groupInviteLinkBtn').on('click', function() {
        showGroupInviteLinkModal();
    });

    $('#leaveGroupBtn').on('click', function() {
        confirmLeaveGroup();
    });

    // Participant management
    $('#addParticipantsBtn').on('click', function() {
        addParticipants();
    });

    // Settings actions
    $('#copyInviteLinkBtn').on('click', function() {
        copyInviteLink();
    });

    $('#resetInviteLinkBtn').on('click', function() {
        resetInviteLink();
    });

    $('#removeGroupPhotoBtn').on('click', function() {
        removeGroupPhoto();
    });
}

function showGroupsManager() {
    // Hide only the main dashboard content, not the header
    $('#mainDashboard').addClass('hidden');
    $('#groupsMainContainer').removeClass('hidden');
    loadGroups();
}

function hideGroupsManager() {
    $('#groupsMainContainer').addClass('hidden');
    // Show only the main dashboard content, not affecting the header
    $('#mainDashboard').removeClass('hidden');
    if (groupsUpdateInterval) {
        clearInterval(groupsUpdateInterval);
        groupsUpdateInterval = null;
    }
}

async function loadGroups() {
    try {
        $('#groupsLoading').addClass('active');
        $('#noGroupsMessage').addClass('hidden');
        
        // Try to get user JID first and ensure it's properly stored
        const userJID = await getUserInfoAndSetJID();
        
        // Also try to get it from the stored token data
        if (!userJID) {
            // Try to get from localStorage with proper parsing
            const storedJID = getLocalStorageItem('currentUserJID');
            if (storedJID) {
                window.currentUserJID = storedJID;
            }
        }
        
        const response = await getGroups();
        
        if (response.code === 200 && response.data && response.data.Groups) {
            groupsCache = response.data;
            displayGroups(response.data.Groups || []);
        } else {
            showError('Failed to load groups: ' + (response.error || 'Unknown error'));
            $('#noGroupsMessage').removeClass('hidden');
        }
    } catch (error) {
        console.error('Error loading groups:', error);
        showError('Error loading groups');
        $('#noGroupsMessage').removeClass('hidden');
    } finally {
        $('#groupsLoading').removeClass('active');
    }
}

function displayGroups(groups) {
    const container = $('#groupsListContainer');
    container.empty();

    if (!groups || groups.length === 0) {
        $('#noGroupsMessage').removeClass('hidden');
        return;
    }

    $('#noGroupsMessage').addClass('hidden');

    groups.forEach(group => {
        const groupItem = createGroupListItem(group);
        container.append(groupItem);
    });
}

// Function to format disappearing timer
function formatDisappearingTimer(seconds) {
    if (!seconds || seconds === 0) return null;
    
    // WhatsApp specific disappearing message options
    switch (seconds) {
        case 86400:     // 24 hours
            return "24 hours";
        case 604800:    // 7 days
            return "7 days";
        case 7776000:   // 90 days
            return "90 days";
        default:
            // Fallback for any other values
            const days = Math.floor(seconds / 86400);
            if (days > 0) {
                return `${days} day${days > 1 ? 's' : ''}`;
            } else {
                const hours = Math.floor(seconds / 3600);
                return `${hours} hour${hours > 1 ? 's' : ''}`;
            }
    }
}

function createGroupListItem(group) {
    const participantCount = group.Participants ? group.Participants.length : 0;
    const currentUserJID = getCurrentUserJID();
    const isAdmin = checkIfUserIsAdmin(group, currentUserJID);
    const isSuperAdmin = checkIfUserIsSuperAdmin(group, currentUserJID);
    
    // Count admins and super admins
    const admins = group.Participants ? group.Participants.filter(p => p.IsAdmin && !p.IsSuperAdmin) : [];
    const superAdmins = group.Participants ? group.Participants.filter(p => p.IsSuperAdmin) : [];
    
    // Get group type information
    const typeInfo = getGroupTypeInfo(group);
    
    // Format creation date
    const createdDate = group.GroupCreated ? new Date(group.GroupCreated).toLocaleDateString('en-US') : 'Unknown';
    
    // Truncate topic if too long
    const topic = group.Topic || 'No description';
    const truncatedTopic = truncateText(topic, 100);
    
    // User role badge
    let userRoleBadge = '';
    if (isSuperAdmin) {
        userRoleBadge = '<span class="ui mini red label"><i class="shield icon"></i>Super Admin</span>';
    } else if (isAdmin) {
        userRoleBadge = '<span class="ui mini orange label"><i class="star icon"></i>Admin</span>';
    }
    
    // Build features badges
    let featureBadges = '';
    if (group.IsAnnounce) {
        featureBadges += '<span class="ui mini grey label"><i class="announcement icon"></i>Admins only</span>';
    }
    if (group.IsLocked) {
        featureBadges += '<span class="ui mini grey label"><i class="lock icon"></i>Locked</span>';
    }
    if (group.IsEphemeral && group.DisappearingTimer) {
        const timerText = formatDisappearingTimer(group.DisappearingTimer);
        featureBadges += `<span class="ui mini grey label"><i class="clock icon"></i>Disappears in ${timerText}</span>`;
    }
    if (group.IsJoinApprovalRequired) {
        featureBadges += '<span class="ui mini grey label"><i class="checkmark icon"></i>Approval required</span>';
    }
    
    // Show linked parent for community groups
    let parentInfo = '';
    if (typeInfo.label === 'Community Group' && group.LinkedParentJID) {
        const parentGroup = groupsCache.Groups ? groupsCache.Groups.find(g => g.JID === group.LinkedParentJID) : null;
        const parentName = parentGroup ? parentGroup.Name : 'Unknown Community';
        parentInfo = `<div class="meta"><i class="sitemap icon"></i>Part of: ${escapeHtml(parentName)}</div>`;
    }
    
    return `
        <div class="item group-list-item" data-group-id="${group.JID}" data-is-admin="${isAdmin}" data-is-super-admin="${isSuperAdmin}" data-group-type="${typeInfo.label}">
            <div class="ui grid">
                <div class="one wide column center aligned">
                    <i class="${typeInfo.icon} large ${typeInfo.color} icon"></i>
                </div>
                <div class="eleven wide column">
                    <div class="content">
                        <div class="header">
                            ${escapeHtml(group.Name || 'Unnamed group')}
                            <span class="ui mini ${typeInfo.color} basic label">${typeInfo.label}</span>
                            ${userRoleBadge}
                        </div>
                        ${parentInfo}
                        <div class="description group-description" data-full-text="${escapeHtml(topic)}">
                            ${escapeHtml(truncatedTopic)}
                            ${topic.length > 100 ? ' <a href="#" class="read-more-link" onclick="toggleDescription(this)">Read more</a>' : ''}
                        </div>
                        <div class="meta">
                            <span><i class="users icon"></i>${participantCount} participants</span>
                            ${admins.length > 0 ? `<span><i class="star icon"></i>${admins.length} admins</span>` : ''}
                            ${superAdmins.length > 0 ? `<span><i class="shield icon"></i>${superAdmins.length} super admins</span>` : ''}
                            <span><i class="calendar icon"></i>Created ${createdDate}</span>
                        </div>
                        <div class="meta">
                            <span><i class="hashtag icon"></i><code>${group.JID}</code></span>
                            <a href="#" onclick="copyGroupId(event, '${group.JID}')"><i class="copy icon"></i>Copy ID</a>
                        </div>
                        <div class="meta">
                            ${featureBadges}
                        </div>
                    </div>
                </div>
                <div class="four wide column right aligned">
                    <div class="ui small buttons">
                        <button class="ui primary button" onclick="viewGroupDetails('${group.JID}')">
                            <i class="eye icon"></i> View
                        </button>
                        ${(isAdmin || isSuperAdmin) ? 
                            `<button class="ui secondary button" onclick="manageGroup('${group.JID}')">
                                <i class="settings icon"></i> Manage
                            </button>` : ''
                        }
                    </div>
                </div>
            </div>
            <div class="ui divider"></div>
        </div>
    `;
}

async function viewGroupDetails(groupJID) {
    try {
        currentGroupId = groupJID;
        
        const response = await getGroupInfo(groupJID);
        
        if (response.code === 200 && response.data) {
            populateGroupDetailsModal(response.data);
            $('#modalGroupDetails').modal('show');
        } else {
            showError('Failed to load group details: ' + (response.error || response.message || 'Unknown error'));
        }
    } catch (error) {
        console.error('Error loading group details:', error);
        showError('Error loading group details: ' + error.message);
    }
}

function populateGroupDetailsModal(groupData) {
    const typeInfo = getGroupTypeInfo(groupData);
    const currentUserJID = getCurrentUserJID();
    const isAdmin = checkIfUserIsAdmin(groupData, currentUserJID);
    const isSuperAdmin = checkIfUserIsSuperAdmin(groupData, currentUserJID);
       
    if (groupData.Participants) {
        groupData.Participants.forEach((p, index) => {
            const participantPhone = extractPhoneFromJID(p.JID);
            const currentUserPhone = extractPhoneFromJID(currentUserJID);
            const isMatch = participantPhone === currentUserPhone;
            if (isMatch) {
                //console.log(`Phone Match: ${currentUserPhone} - ${participantPhone}`);
                //console.log(`  ${index + 1}. JID: ${p.JID}, Phone: ${participantPhone}, IsAdmin: ${p.IsAdmin}, IsSuperAdmin: ${p.IsSuperAdmin}`);
            }
        });
    }
    
    $('#groupDetailsTitle').text(groupData.Name || 'Group Details');
    $('#groupDetailsName').text(groupData.Name || 'Unnamed group');
    $('#groupDetailsDescription').text(groupData.Topic || 'No description');
    $('#groupDetailsParticipants').text((groupData.Participants || []).length + ' participants');
    
    // Format creation date
    const createdDate = groupData.GroupCreated ? new Date(groupData.GroupCreated).toLocaleDateString('en-US', {
        year: 'numeric',
        month: 'long',
        day: 'numeric'
    }) : 'Unknown';
    $('#groupDetailsCreated').text(createdDate);

    // Show group type and features
    let featuresText = `${typeInfo.label}`;
    if (groupData.IsAnnounce) featuresText += ', Admins only';
    if (groupData.IsLocked) featuresText += ', Locked';
    if (groupData.IsEphemeral && groupData.DisappearingTimer) {
        const timerText = formatDisappearingTimer(groupData.DisappearingTimer);
        featuresText += `, Disappearing messages (${timerText})`;
    }
    if (groupData.IsJoinApprovalRequired) featuresText += ', Approval required';
    
    // Add group features info
    if ($('#groupDetailsFeatures').length === 0) {
        $('#groupDetailsCreated').parent().after(`
            <div class="item">
                <div class="header">Type & Features</div>
                <div id="groupDetailsFeatures">${featuresText}</div>
            </div>
        `);
    } else {
        $('#groupDetailsFeatures').text(featuresText);
    }

    // Show linked parent for community groups
    if (typeInfo.label === 'Community Group' && groupData.LinkedParentJID) {
        const parentGroup = groupsCache.Groups ? groupsCache.Groups.find(g => g.JID === groupData.LinkedParentJID) : null;
        const parentName = parentGroup ? parentGroup.Name : 'Unknown Community';
        
        if ($('#groupDetailsParent').length === 0) {
            $('#groupDetailsFeatures').parent().after(`
                <div class="item">
                    <div class="header">Parent Community</div>
                    <div id="groupDetailsParent">${escapeHtml(parentName)}</div>
                </div>
            `);
        } else {
            $('#groupDetailsParent').text(parentName);
        }
    } else {
        $('#groupDetailsParent').parent().remove();
    }

    // Show/hide admin actions based on user permissions
    const adminSection = $('#adminActionsSection');
    
    if (isAdmin || isSuperAdmin) {
        adminSection.show();
    } else {
        adminSection.hide();
    }

    // Always show placeholder since no photo URL is provided by API
    $('#groupDetailsPhoto').hide();
    $('#groupDetailsNoPhoto').show();

    // Participants list
    const participantsList = $('#groupParticipantsList');
    participantsList.empty();

    if (groupData.Participants && groupData.Participants.length > 0) {
        groupData.Participants.forEach(participant => {
            const phone = participant.JID ? participant.JID.split('@')[0] : 'Unknown';
            const participantIsAdmin = participant.IsAdmin || false;
            const participantIsSuperAdmin = participant.IsSuperAdmin || false;
            const currentUserPhone = extractPhoneFromJID(currentUserJID);
            const participantPhone = extractPhoneFromJID(participant.JID);
            const isCurrentUser = currentUserPhone && participantPhone && currentUserPhone === participantPhone;
            
            let actionButtons = '';
            
            // Show action buttons only if current user is admin/super admin and target is not current user
            if (!isCurrentUser && (isAdmin || isSuperAdmin)) {
                const buttons = [];
                
                // Remove button - available for admins
                buttons.push(`
                    <button class="ui mini red button" onclick="removeParticipant('${participant.JID}')" title="Remove from group">
                        <i class="minus icon"></i> Remove
                    </button>
                `);
                
                // Promote/Demote buttons - based on current user permissions
                if (isSuperAdmin) {
                    // Super admins can promote/demote anyone
                    if (!participantIsAdmin && !participantIsSuperAdmin) {
                        // Promote to admin
                        buttons.push(`
                            <button class="ui mini orange button" onclick="promoteParticipant('${participant.JID}')" title="Promote to admin">
                                <i class="star icon"></i> Promote
                            </button>
                        `);
                    } else if (participantIsAdmin && !participantIsSuperAdmin) {
                        // Demote from admin
                        buttons.push(`
                            <button class="ui mini grey button" onclick="demoteParticipant('${participant.JID}')" title="Demote from admin">
                                <i class="star outline icon"></i> Demote
                            </button>
                        `);
                    }
                } else if (isAdmin && !participantIsAdmin && !participantIsSuperAdmin) {
                    // Regular admins can only promote regular users to admin
                    buttons.push(`
                        <button class="ui mini orange button" onclick="promoteParticipant('${participant.JID}')" title="Promote to admin">
                            <i class="star icon"></i> Promote
                        </button>
                    `);
                }
                
                if (buttons.length > 0) {
                    actionButtons = `
                        <div class="ui mini buttons">
                            ${buttons.join('')}
                        </div>
                    `;
                }
            }
            
            const participantItem = `
                <div class="item participant-item">
                    <div class="content">
                        <div class="participant-info">
                            <div class="header">${escapeHtml(participant.DisplayName || phone)}</div>
                            <div class="description">+${phone}</div>
                        </div>
                        <div class="participant-actions">
                            <div class="participant-labels">
                                ${participantIsSuperAdmin ? '<span class="ui mini red label"><i class="shield icon"></i>Super Admin</span>' : 
                                  participantIsAdmin ? '<span class="ui mini orange label"><i class="star icon"></i>Admin</span>' : ''}
                                ${isCurrentUser ? '<span class="ui mini blue label">You</span>' : ''}
                            </div>
                            ${actionButtons}
                        </div>
                    </div>
                </div>
            `;
            participantsList.append(participantItem);
        });
    } else {
        participantsList.append('<div class="item">No participants found</div>');
    }
}

function createParticipantItem(participant) {
    const phone = participant.JID ? participant.JID.split('@')[0] : 'Unknown';
    const isAdmin = participant.IsAdmin || false;
    const isSuperAdmin = participant.IsSuperAdmin || false;
    
    // Get current user JID to check if this participant is the current user
    const currentUserJID = getCurrentUserJID();
    const currentUserPhone = extractPhoneFromJID(currentUserJID);
    const participantPhone = extractPhoneFromJID(participant.JID);
    const isCurrentUser = currentUserPhone && participantPhone && currentUserPhone === participantPhone;
    
    return `
        <div class="item participant-item">
            <div class="content">
                <div class="participant-info">
                    <div class="header">${escapeHtml(participant.DisplayName || phone)}</div>
                    <div class="description">+${phone}</div>
                </div>
                <div class="participant-actions">
                    <div class="participant-labels">
                        ${isSuperAdmin ? '<span class="ui mini red label"><i class="shield icon"></i>Super Admin</span>' : 
                          isAdmin ? '<span class="ui mini orange label"><i class="star icon"></i>Admin</span>' : ''}
                        ${isCurrentUser ? '<span class="ui mini blue label">You</span>' : ''}
                    </div>
                </div>
            </div>
        </div>
    `;
}

function copyGroupId(event, jid) {
    event.preventDefault();
    navigator.clipboard.writeText(jid).then(() => {
        $.toast({ class: 'success', message: 'Group ID copied to clipboard!' });
    }).catch(() => {
        $.toast({ class: 'error', message: 'Failed to copy group ID' });
    });
}

function filterGroups() {
    const searchTerm = $('#groupsSearch').val().toLowerCase();
    const filterType = $('#groupsFilter').val();
    
    $('.group-list-item').each(function() {
        const item = $(this);
        const groupName = item.find('.header').first().text().toLowerCase();
        const groupDescription = item.find('.group-description').attr('data-full-text').toLowerCase();
        const groupType = item.attr('data-group-type').toLowerCase();
        
        let showItem = true;
        
        // Apply search filter
        if (searchTerm && !groupName.includes(searchTerm) && !groupDescription.includes(searchTerm)) {
            showItem = false;
        }
        
        // Apply type filter
        if (filterType === 'community' && !groupType.includes('community')) {
            showItem = false;
        } else if (filterType === 'normal' && groupType !== 'group') {
            showItem = false;
        } else if (filterType === 'community_group' && groupType !== 'community group') {
            showItem = false;
        }
        
        if (showItem) {
            item.show();
        } else {
            item.hide();
        }
    });
}

function showCreateGroupModal() {
    // Reset modal state
    const statusDiv = $('#contactsStatus');
    const statusText = $('#contactsStatusText');
    const dropdown = $('#participantsDropdown');
    
    // Clear previous contacts cache to force fresh load
    contactsCache = {};
    
    // Reset dropdown
    dropdown.dropdown('clear');
    dropdown.find('.menu').html('<div class="item disabled">No contacts loaded</div>');
    
    // Show initial status message
    statusDiv.show().removeClass('success error').addClass('info');
    statusText.html('<i class="info circle icon"></i> Click "Load Contacts" to populate the participants list.');
    
    $('#modalCreateGroup').modal('show');
}

async function loadContacts() {
    try {
        const response = await getContactsForGroups();
        if (response && Array.isArray(response)) {
            // Convert array format to object format expected by populateParticipantsDropdown
            contactsCache = {};
            response.forEach(contact => {
                const jid = contact.Phone + '@s.whatsapp.net';
                contactsCache[jid] = {
                    FullName: contact.FullName,
                    PushName: contact.PushName,
                    Phone: contact.Phone
                };
            });
        }
    } catch (error) {
        console.error('Error loading contacts:', error);
    }
}

function populateParticipantsDropdown() {
    const dropdown = $('#participantsDropdown .menu');
    dropdown.empty();
    
    if (contactsCache && typeof contactsCache === 'object' && Object.keys(contactsCache).length > 0) {
        Object.entries(contactsCache).forEach(([jid, contact]) => {
            const phone = contact.Phone;
            const name = contact.FullName || contact.PushName || phone;
            dropdown.append(`<div class="item" data-value="${phone}">${escapeHtml(name)} (+${phone})</div>`);
        });
        
        // Update dropdown placeholder
        $('#participantsDropdown .default.text').text('Select participants');
    } else {
        dropdown.append('<div class="item disabled">No contacts loaded. Click "Load Contacts" to populate the list.</div>');
        $('#participantsDropdown .default.text').text('Load contacts first');
    }
    
    $('#participantsDropdown').dropdown('refresh');
}

async function createGroup() {
    try {
        const form = $('#createGroupForm');
        const formData = new FormData(form[0]);
        
        const groupName = formData.get('groupName');
        const participants = $('#participantsDropdown').dropdown('get value');
        
        if (!groupName || !participants || participants.length === 0) {
            showError('Please fill in all required fields');
            return;
        }
        
        // Convert participants to array of phone numbers (API expects array of strings)
        let participantNumbers;
        if (Array.isArray(participants)) {
            participantNumbers = participants;
        } else if (typeof participants === 'string') {
            // If it's a single string, split by comma and clean up
            participantNumbers = participants.split(',').map(p => p.trim()).filter(p => p.length > 0);
        } else {
            participantNumbers = [participants.toString()];
        }
        
        const createData = {
            name: groupName,
            participants: participantNumbers
        };
        
        const response = await createGroupAPI(createData);
        
        if (response.success || response.code === 200) {
            showSuccess('Group created successfully!');
            $('#modalCreateGroup').modal('hide');
            form[0].reset();
            $('#participantsDropdown').dropdown('clear');
            loadGroups(); // Refresh groups list
        } else {
            showError('Failed to create group: ' + (response.error || 'Unknown error'));
        }
    } catch (error) {
        console.error('Error creating group:', error);
        showError('Error creating group');
    }
}

function showJoinGroupModal() {
    $('#modalJoinGroup').modal('show');
}

async function previewGroup() {
    try {
        const inviteCode = $('input[name="inviteCode"]').val().trim();
        if (!inviteCode) {
            showError('Please enter an invite code or link');
            return;
        }
        
        // Extract code from full link if needed
        const code = extractInviteCode(inviteCode);
        
        const response = await getGroupInviteInfo(code);
        
        if (response.success && response.data) {
            displayGroupPreview(response.data);
        } else {
            showError('Failed to get group preview: ' + (response.error || 'Invalid invite code'));
        }
    } catch (error) {
        console.error('Error previewing group:', error);
        showError('Error previewing group');
    }
}

function displayGroupPreview(groupData) {
    const previewContainer = $('#groupPreviewContainer');
    const previewContent = $('#groupPreviewContent');
    
    previewContent.html(`
        <div class="ui grid">
            <div class="four wide column">
                <div class="ui icon header">
                    <i class="users icon"></i>
                </div>
            </div>
            <div class="twelve wide column">
                <h4>${escapeHtml(groupData.Name || 'Unnamed Group')}</h4>
                <p>${escapeHtml(groupData.Topic || 'No description')}</p>
                <p><strong>Participants:</strong> ${groupData.Size || 0}</p>
            </div>
        </div>
    `);
    
    previewContainer.removeClass('hidden');
}

async function joinGroup() {
    try {
        const inviteCode = $('input[name="inviteCode"]').val().trim();
        if (!inviteCode) {
            showError('Please enter an invite code or link');
            return;
        }
        
        const code = extractInviteCode(inviteCode);
        
        const response = await joinGroupAPI(code);
        
        if (response.success) {
            showSuccess('Successfully joined the group!');
            $('#modalJoinGroup').modal('hide');
            $('#joinGroupForm')[0].reset();
            $('#groupPreviewContainer').addClass('hidden');
            loadGroups(); // Refresh groups list
        } else {
            showError('Failed to join group: ' + (response.error || 'Unknown error'));
        }
    } catch (error) {
        console.error('Error joining group:', error);
        showError('Error joining group');
    }
}

function extractInviteCode(input) {
    // Extract code from WhatsApp invite link or return as-is if it's just a code
    const match = input.match(/chat\.whatsapp\.com\/([A-Za-z0-9]+)/);
    return match ? match[1] : input;
}

// API Functions
async function getGroups() {
    const token = getLocalStorageItem('token');
    const myHeaders = new Headers();
    myHeaders.append('token', token);
    myHeaders.append('Content-Type', 'application/json');
    
    const response = await fetch(baseUrl + "/group/list", {
        method: "GET",
        headers: myHeaders
    });
    
    return await response.json();
}

async function getGroupInfo(groupJID) {
    const token = getLocalStorageItem('token');
    const myHeaders = new Headers();
    myHeaders.append('token', token);
    myHeaders.append('Content-Type', 'application/json');
    
    const response = await fetch(baseUrl + "/group/info?" + new URLSearchParams({
        groupJID: groupJID
    }), {
        method: "GET",
        headers: myHeaders
    });
    
    return await response.json();
}

async function createGroupAPI(groupData) {
    const token = getLocalStorageItem('token');
    const myHeaders = new Headers();
    myHeaders.append('token', token);
    myHeaders.append('Content-Type', 'application/json');
    
    const response = await fetch(baseUrl + "/group/create", {
        method: "POST",
        headers: myHeaders,
        body: JSON.stringify(groupData)
    });
    
    return await response.json();
}

async function getGroupInviteInfo(code) {
    const token = getLocalStorageItem('token');
    const myHeaders = new Headers();
    myHeaders.append('token', token);
    myHeaders.append('Content-Type', 'application/json');
    
    const response = await fetch(baseUrl + "/group/inviteinfo", {
        method: "POST",
        headers: myHeaders,
        body: JSON.stringify({ code: code })
    });
    
    return await response.json();
}

async function joinGroupAPI(code) {
    const token = getLocalStorageItem('token');
    const myHeaders = new Headers();
    myHeaders.append('token', token);
    myHeaders.append('Content-Type', 'application/json');
    
    const response = await fetch(baseUrl + "/group/join", {
        method: "POST",
        headers: myHeaders,
        body: JSON.stringify({ code: code })
    });
    
    return await response.json();
}

// Utility functions
function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

function getCurrentUserJID() {
    // Try to get current user JID from various sources
    let userJID = window.currentUserJID;

    // If not found in window, try to get from localStorage using the helper function
    if (!userJID) {
        userJID = getLocalStorageItem('currentUserJID');
        
        // If found, also set it in window for future use
        if (userJID) {
            window.currentUserJID = userJID;
        }
    }
    
    // If still not found, try direct localStorage access as fallback
    if (!userJID) {
        try {
            const storedItem = localStorage.getItem('currentUserJID');
            if (storedItem) {
                // Try to parse as JSON first (expiry format)
                try {
                    const parsed = JSON.parse(storedItem);
                    if (parsed && parsed.value) {
                        userJID = parsed.value;
                    }
                } catch (e) {
                    // If JSON parsing fails, treat as plain string
                    userJID = storedItem;
                }
                
                // Set in window for future use
                if (userJID) {
                    window.currentUserJID = userJID;
                }
            }
        } catch (e) {
            console.log('Error accessing localStorage currentUserJID:', e);
        }
    }
    
    //console.log('Final userJID:', userJID);
    
    // Return empty string if still not found (this will make admin checks return false)
    return userJID || '';
}

// Helper function to extract phone number from JID
function extractPhoneFromJID(jid) {
    if (!jid) return '';
    
    // Extract the phone number part before the first ':' or '@'
    const phone = jid.split(':')[0].split('@')[0];
       
    return phone;
}

function checkIfUserIsAdmin(group, userJID) {
    // Return false if no participants data or no userJID
    if (!group.Participants || !userJID) {
        return false;
    }
    
    // Extract phone number from current user JID
    const currentUserPhone = extractPhoneFromJID(userJID);
    if (!currentUserPhone) return false;
    
    // Find the current user in the participants list by comparing phone numbers
    const participant = group.Participants.find(p => {
        const participantPhone = extractPhoneFromJID(p.JID);
        return participantPhone === currentUserPhone;
    });
    
    // Return true if user is found and is either admin or super admin
    return participant ? (participant.IsAdmin || participant.IsSuperAdmin) : false;
}

function checkIfUserIsSuperAdmin(group, userJID) {
    // Return false if no participants data or no userJID
    if (!group.Participants || !userJID) {
        return false;
    }
    
    // Extract phone number from current user JID
    const currentUserPhone = extractPhoneFromJID(userJID);
    if (!currentUserPhone) return false;
    
    // Find the current user in the participants list by comparing phone numbers
    const participant = group.Participants.find(p => {
        const participantPhone = extractPhoneFromJID(p.JID);
        return participantPhone === currentUserPhone;
    });
    
    // Return true only if user is found and is specifically a super admin
    return participant ? participant.IsSuperAdmin : false;
}

function truncateText(text, maxLength) {
    if (!text || text.length <= maxLength) return text;
    return text.substring(0, maxLength) + '...';
}

function toggleDescription(element) {
    const descriptionDiv = element.parentElement;
    const fullText = descriptionDiv.getAttribute('data-full-text');
    
    if (element.textContent.trim() === 'Read more') {
        descriptionDiv.innerHTML = escapeHtml(fullText) + ' <a href="#" class="read-more-link" onclick="toggleDescription(this)">Read less</a>';
    } else {
        const truncated = truncateText(fullText, 100);
        descriptionDiv.innerHTML = escapeHtml(truncated) + ' <a href="#" class="read-more-link" onclick="toggleDescription(this)">Read more</a>';
    }
}

function openGroupChat(groupJID) {
    // This would open the chat interface for the group
    // Implementation depends on your chat interface
    //console.log('Opening chat for group:', groupJID);
    showSuccess('Chat functionality would be opened here');
}

function manageGroup(groupJID) {
    viewGroupDetails(groupJID);
}

// Additional functions for group management will be added in subsequent tasks
// This includes: edit group, manage participants, group settings, etc.

// Enhanced function to get user info and set userJID
async function getUserInfoAndSetJID() {
    try {
        // Try to get user info from status API to get the JID
        const token = getLocalStorageItem('token');
        const myHeaders = new Headers();
        myHeaders.append('token', token);
        myHeaders.append('Content-Type', 'application/json');
        
        const response = await fetch(baseUrl + "/session/status", {
            method: "GET",
            headers: myHeaders
        });
        
        const data = await response.json();
        if (data.code === 200 && data.data && data.data.jid) {
            const userJID = data.data.jid;
            localStorage.setItem('currentUserJID', userJID);
            window.currentUserJID = userJID;
            //console.log('User JID set from status:', userJID);
            return userJID;
        }
    } catch (error) {
        console.log('Could not get user status:', error);
    }
    
    return null;
}

// Group management functions
function showEditGroupInfoModal() {
    if (!currentGroupId) return;
    
    // Get current group data
    const groupData = groupsCache.Groups ? groupsCache.Groups.find(g => g.JID === currentGroupId) : null;
    if (groupData) {
        $('#editGroupNameInput').val(groupData.Name || '');
        $('#editGroupDescriptionInput').val(groupData.Topic || '');
    }
    
    $('#modalEditGroupInfo').modal('show');
}

function showGroupSettingsModal() {
    if (!currentGroupId) return;
    
    // Get current group data
    const groupData = groupsCache.Groups ? groupsCache.Groups.find(g => g.JID === currentGroupId) : null;
    if (groupData) {
        // Set current settings
        $('#announceOnlyToggle').checkbox(groupData.IsAnnounce ? 'check' : 'uncheck');
        $('#editInfoToggle').checkbox(groupData.IsLocked ? 'check' : 'uncheck');
        
        // Set disappearing timer
        let timerValue = 'off';
        if (groupData.IsEphemeral && groupData.DisappearingTimer) {
            switch (groupData.DisappearingTimer) {
                case 86400: timerValue = '24h'; break;
                case 604800: timerValue = '7d'; break;
                case 7776000: timerValue = '90d'; break;
            }
        }
        $('#disappearingTimerSelect').dropdown('set selected', timerValue);
    }
    
    $('#modalGroupSettings').modal('show');
}

function showGroupInviteLinkModal() {
    if (!currentGroupId) return;
    
    $('#currentInviteLink').val('Loading...');
    $('#modalGroupInviteLink').modal('show');
    
    // Load current invite link
    getGroupInviteLink(currentGroupId).then(response => {
        if (response.success && response.data && response.data.InviteLink) {
            $('#currentInviteLink').val(response.data.InviteLink);
        } else {
            $('#currentInviteLink').val('Failed to load invite link');
        }
    }).catch(error => {
        console.error('Error loading invite link:', error);
        $('#currentInviteLink').val('Error loading invite link');
    });
}

function showManageParticipantsModal() {
    if (!currentGroupId) return;
    
    // Clear previous data
    $('#addParticipantsDropdown').dropdown('clear');
    $('#participantsContactsStatus').hide();
    
    // Populate current participants
    populateCurrentParticipantsList();
    
    $('#modalManageParticipants').modal('show');
}

function populateCurrentParticipantsList() {
    const participantsList = $('#manageParticipantsList');
    participantsList.empty();
    
    const groupData = groupsCache.Groups ? groupsCache.Groups.find(g => g.JID === currentGroupId) : null;
    if (groupData && groupData.Participants) {
        const currentUserJID = getCurrentUserJID();
        const currentUserIsSuperAdmin = checkIfUserIsSuperAdmin(groupData, currentUserJID);
        const currentUserIsAdmin = checkIfUserIsAdmin(groupData, currentUserJID);
        
        groupData.Participants.forEach(participant => {
            const phone = participant.JID ? participant.JID.split('@')[0] : 'Unknown';
            const participantIsAdmin = participant.IsAdmin || false;
            const participantIsSuperAdmin = participant.IsSuperAdmin || false;
            const currentUserPhone = extractPhoneFromJID(currentUserJID);
            const participantPhone = extractPhoneFromJID(participant.JID);
            const isCurrentUser = currentUserPhone && participantPhone && currentUserPhone === participantPhone;
            
            let actionButtons = '';
            
            // Show action buttons only if current user is admin/super admin and target is not current user
            if (!isCurrentUser && (currentUserIsAdmin || currentUserIsSuperAdmin)) {
                const buttons = [];
                
                // Remove button - available for admins
                buttons.push(`
                    <button class="ui mini red button" onclick="removeParticipant('${participant.JID}')" title="Remove from group">
                        <i class="minus icon"></i> Remove
                    </button>
                `);
                
                // Promote/Demote buttons - based on current user permissions
                if (currentUserIsSuperAdmin) {
                    // Super admins can promote/demote anyone
                    if (!participantIsAdmin && !participantIsSuperAdmin) {
                        // Promote to admin
                        buttons.push(`
                            <button class="ui mini orange button" onclick="promoteParticipant('${participant.JID}')" title="Promote to admin">
                                <i class="star icon"></i> Promote
                            </button>
                        `);
                    } else if (participantIsAdmin && !participantIsSuperAdmin) {
                        // Demote from admin
                        buttons.push(`
                            <button class="ui mini grey button" onclick="demoteParticipant('${participant.JID}')" title="Demote from admin">
                                <i class="star outline icon"></i> Demote
                            </button>
                        `);
                    }
                } else if (currentUserIsAdmin && !participantIsAdmin && !participantIsSuperAdmin) {
                    // Regular admins can only promote regular users to admin
                    buttons.push(`
                        <button class="ui mini orange button" onclick="promoteParticipant('${participant.JID}')" title="Promote to admin">
                            <i class="star icon"></i> Promote
                        </button>
                    `);
                }
                
                if (buttons.length > 0) {
                    actionButtons = `
                        <div class="ui mini buttons">
                            ${buttons.join('')}
                        </div>
                    `;
                }
            }
            
            const participantItem = `
                <div class="item participant-item">
                    <div class="content">
                        <div class="participant-info">
                            <div class="header">${escapeHtml(participant.DisplayName || phone)}</div>
                            <div class="description">+${phone}</div>
                        </div>
                        <div class="participant-actions">
                            <div class="participant-labels">
                                ${participantIsSuperAdmin ? '<span class="ui mini red label"><i class="shield icon"></i>Super Admin</span>' : 
                                  participantIsAdmin ? '<span class="ui mini orange label"><i class="star icon"></i>Admin</span>' : ''}
                                ${isCurrentUser ? '<span class="ui mini blue label">You</span>' : ''}
                            </div>
                            ${actionButtons}
                        </div>
                    </div>
                </div>
            `;
            participantsList.append(participantItem);
        });
    }
}

function populateAddParticipantsDropdown() {
    const dropdown = $('#addParticipantsDropdown .menu');
    dropdown.empty();
    
    if (contactsCache && typeof contactsCache === 'object' && Object.keys(contactsCache).length > 0) {
        // Get current participants to exclude them
        const groupData = groupsCache.Groups ? groupsCache.Groups.find(g => g.JID === currentGroupId) : null;
        const currentParticipants = groupData && groupData.Participants ? 
            groupData.Participants.map(p => p.JID.split('@')[0]) : [];
        
        Object.entries(contactsCache).forEach(([jid, contact]) => {
            const phone = contact.Phone;
            const name = contact.FullName || contact.PushName || phone;
            
            // Don't show contacts that are already participants
            if (!currentParticipants.includes(phone)) {
                dropdown.append(`<div class="item" data-value="${phone}">${escapeHtml(name)} (+${phone})</div>`);
            }
        });
        
        $('#addParticipantsDropdown .default.text').text('Select contacts to add');
    } else {
        dropdown.append('<div class="item disabled">No contacts loaded. Click "Load Contacts" to populate the list.</div>');
        $('#addParticipantsDropdown .default.text').text('Load contacts first');
    }
    
    $('#addParticipantsDropdown').dropdown('refresh');
}

// Action functions
async function saveGroupInfo() {
    if (!currentGroupId) return;
    
    const groupName = $('#editGroupNameInput').val().trim();
    const groupDescription = $('#editGroupDescriptionInput').val().trim();
    
    if (!groupName) {
        showError('Group name is required');
        return;
    }
    
    try {
        // Update group name
        const nameResponse = await updateGroupName(currentGroupId, groupName);
        if (!nameResponse.success) {
            throw new Error('Failed to update group name: ' + (nameResponse.error || 'Unknown error'));
        }
        
        // Update group description if provided
        if (groupDescription) {
            const topicResponse = await updateGroupTopic(currentGroupId, groupDescription);
            if (!topicResponse.success) {
                throw new Error('Failed to update group description: ' + (topicResponse.error || 'Unknown error'));
            }
        }
        
        showSuccess('Group information updated successfully!');
        $('#modalEditGroupInfo').modal('hide');
        
        // Refresh group details
        setTimeout(() => {
            viewGroupDetails(currentGroupId);
        }, 1000);
        
    } catch (error) {
        console.error('Error updating group info:', error);
        showError('Error updating group information: ' + error.message);
    }
}

async function saveGroupSettings() {
    if (!currentGroupId) return;
    
    try {
        const announceOnly = $('#announceOnlyToggle').checkbox('is checked');
        const locked = $('#editInfoToggle').checkbox('is checked');
        const disappearingTimer = $('#disappearingTimerSelect').dropdown('get value');
        
        // Update announce setting
        const announceResponse = await updateGroupAnnounce(currentGroupId, announceOnly);
        if (!announceResponse.success) {
            throw new Error('Failed to update announce setting');
        }
        
        // Update locked setting
        const lockedResponse = await updateGroupLocked(currentGroupId, locked);
        if (!lockedResponse.success) {
            throw new Error('Failed to update locked setting');
        }
        
        // Update disappearing timer
        const ephemeralResponse = await updateGroupEphemeral(currentGroupId, disappearingTimer);
        if (!ephemeralResponse.success) {
            throw new Error('Failed to update disappearing timer');
        }
        
        showSuccess('Group settings updated successfully!');
        $('#modalGroupSettings').modal('hide');
        
        // Refresh group details
        setTimeout(() => {
            viewGroupDetails(currentGroupId);
        }, 1000);
        
    } catch (error) {
        console.error('Error updating group settings:', error);
        showError('Error updating group settings: ' + error.message);
    }
}

async function addParticipants() {
    if (!currentGroupId) return;
    
    const selectedParticipants = $('#addParticipantsDropdown').dropdown('get value');
    if (!selectedParticipants || selectedParticipants.length === 0) {
        showError('Please select participants to add');
        return;
    }
    
    try {
        // Convert phone numbers - API expects just phone numbers, not JIDs
        const participantNumbers = Array.isArray(selectedParticipants) ? 
            selectedParticipants : [selectedParticipants];
        
        const response = await updateGroupParticipants(currentGroupId, 'add', participantNumbers);
        
        if (response.success) {
            showSuccess('Participants added successfully!');
            $('#addParticipantsDropdown').dropdown('clear');
            
            // Refresh participants list
            populateCurrentParticipantsList();
            populateAddParticipantsDropdown();
            
            // Refresh group details
            setTimeout(() => {
                viewGroupDetails(currentGroupId);
            }, 1000);
        } else {
            throw new Error(response.error || 'Unknown error');
        }
        
    } catch (error) {
        console.error('Error adding participants:', error);
        showError('Error adding participants: ' + error.message);
    }
}

async function removeParticipant(participantJID) {
    if (!currentGroupId || !participantJID) return;
    
    if (!confirm('Are you sure you want to remove this participant from the group?')) {
        return;
    }
    
    try {
        // Extract phone number from JID (remove @s.whatsapp.net part)
        const phoneNumber = participantJID.split('@')[0];
        
        const response = await updateGroupParticipants(currentGroupId, 'remove', [phoneNumber]);
        
        if (response.success) {
            showSuccess('Participant removed successfully!');
            
            // Refresh participants list
            populateCurrentParticipantsList();
            populateAddParticipantsDropdown();
            
            // Refresh group details
            setTimeout(() => {
                viewGroupDetails(currentGroupId);
            }, 1000);
        } else {
            throw new Error(response.error || 'Unknown error');
        }
        
    } catch (error) {
        console.error('Error removing participant:', error);
        showError('Error removing participant: ' + error.message);
    }
}

async function promoteParticipant(participantJID) {
    if (!currentGroupId || !participantJID) return;
    
    if (!confirm('Are you sure you want to promote this participant to admin?')) {
        return;
    }
    
    try {
        // Extract phone number from JID (remove @s.whatsapp.net part)
        const phoneNumber = participantJID.split('@')[0];
        
        const response = await updateGroupParticipants(currentGroupId, 'promote', [phoneNumber]);
        
        if (response.success) {
            showSuccess('Participant promoted to admin successfully!');
            
            // Refresh participants list
            populateCurrentParticipantsList();
            
            // Refresh group details
            setTimeout(() => {
                viewGroupDetails(currentGroupId);
            }, 1000);
        } else {
            throw new Error(response.error || 'Unknown error');
        }
        
    } catch (error) {
        console.error('Error promoting participant:', error);
        showError('Error promoting participant: ' + error.message);
    }
}

async function demoteParticipant(participantJID) {
    if (!currentGroupId || !participantJID) return;
    
    if (!confirm('Are you sure you want to demote this admin to regular participant?')) {
        return;
    }
    
    try {
        // Extract phone number from JID (remove @s.whatsapp.net part)
        const phoneNumber = participantJID.split('@')[0];
        
        const response = await updateGroupParticipants(currentGroupId, 'demote', [phoneNumber]);
        
        if (response.success) {
            showSuccess('Admin demoted to participant successfully!');
            
            // Refresh participants list
            populateCurrentParticipantsList();
            
            // Refresh group details
            setTimeout(() => {
                viewGroupDetails(currentGroupId);
            }, 1000);
        } else {
            throw new Error(response.error || 'Unknown error');
        }
        
    } catch (error) {
        console.error('Error demoting participant:', error);
        showError('Error demoting participant: ' + error.message);
    }
}

async function copyInviteLink() {
    const inviteLink = $('#currentInviteLink').val();
    if (!inviteLink || inviteLink === 'Loading...' || inviteLink.includes('Failed') || inviteLink.includes('Error')) {
        showError('No valid invite link to copy');
        return;
    }
    
    try {
        await navigator.clipboard.writeText(inviteLink);
        showSuccess('Invite link copied to clipboard!');
    } catch (error) {
        console.error('Error copying to clipboard:', error);
        showError('Failed to copy invite link');
    }
}

async function resetInviteLink() {
    if (!currentGroupId) return;
    
    if (!confirm('Are you sure you want to reset the invite link? This will invalidate the current link.')) {
        return;
    }
    
    try {
        $('#currentInviteLink').val('Resetting...');
        
        const response = await getGroupInviteLink(currentGroupId, true); // true = reset
        
        if (response.success && response.data && response.data.InviteLink) {
            $('#currentInviteLink').val(response.data.InviteLink);
            showSuccess('Invite link reset successfully!');
        } else {
            throw new Error(response.error || 'Unknown error');
        }
        
    } catch (error) {
        console.error('Error resetting invite link:', error);
        $('#currentInviteLink').val('Error resetting invite link');
        showError('Error resetting invite link: ' + error.message);
    }
}

async function removeGroupPhoto() {
    if (!currentGroupId) return;
    
    if (!confirm('Are you sure you want to remove the group photo?')) {
        return;
    }
    
    try {
        const response = await removeGroupPhotoAPI(currentGroupId);
        
        if (response.success) {
            showSuccess('Group photo removed successfully!');
            
            // Refresh group details
            setTimeout(() => {
                viewGroupDetails(currentGroupId);
            }, 1000);
        } else {
            throw new Error(response.error || 'Unknown error');
        }
        
    } catch (error) {
        console.error('Error removing group photo:', error);
        showError('Error removing group photo: ' + error.message);
    }
}

async function uploadGroupPhoto(file) {
    if (!currentGroupId || !file) {
        showError('No group selected or file provided');
        return;
    }
    
    // Validate file type
    const supportedTypes = ['image/jpeg', 'image/jpg', 'image/png', 'image/gif', 'image/webp'];
    if (!file.type.startsWith('image/') || !supportedTypes.includes(file.type.toLowerCase())) {
        showError('Please select a supported image file (JPEG, PNG, GIF, WebP)');
        return;
    }
    
    // Validate file size (max 10MB)
    const maxSize = 10 * 1024 * 1024; // 10MB
    if (file.size > maxSize) {
        showError('Image file is too large. Please select an image smaller than 10MB.');
        return;
    }
    
    let uploadButton = null;
    
    try {
        // Show loading state
        uploadButton = document.querySelector('#changeGroupPhotoBtn');
        if (uploadButton) {
            uploadButton.disabled = true;
            uploadButton.innerHTML = '<i class="spinner loading icon"></i> Processing...';
        }
        
        console.log('Starting group photo upload process...');
        console.log('File details:', {
            name: file.name,
            type: file.type,
            size: file.size,
            lastModified: file.lastModified
        });
        
        // Convert file to base64 (preserves format when possible)
        console.log('Converting image to optimized format...');
        const processedImage = await fileToBase64(file);
        
        if (!processedImage) {
            throw new Error('Failed to process image file');
        }
        
        console.log('Image processed successfully, uploading to server...');
        console.log('Processed image stats:', {
            base64Length: processedImage.base64.length,
            format: processedImage.format,
            estimatedSizeKB: Math.round(processedImage.base64.length * 0.75 / 1024),
            firstChars: processedImage.base64.substring(0, 20),
            lastChars: processedImage.base64.substring(processedImage.base64.length - 20)
        });
        
        // Update button text
        if (uploadButton) {
            uploadButton.innerHTML = '<i class="spinner loading icon"></i> Uploading...';
        }
        
        const response = await updateGroupPhoto(currentGroupId, processedImage.base64);
        
        if (response.success) {
            showSuccess('Group photo updated successfully!');
            console.log('Group photo upload completed successfully');
            
            // Refresh group details after a short delay
            setTimeout(() => {
                viewGroupDetails(currentGroupId);
            }, 1500);
        } else {
            const errorMsg = response.error || 'Failed to update group photo';
            console.error('Server error:', errorMsg);
            
            // If the error suggests a format issue, provide helpful feedback
            if (errorMsg.includes('decode') || errorMsg.includes('base64')) {
                throw new Error('Image encoding error. Please try a different image file.');
            } else if (errorMsg.includes('format') || errorMsg.includes('data:image')) {
                throw new Error('Image format error. Please ensure the image is in a supported format (JPEG, PNG, GIF, WebP).');
            } else if (errorMsg.includes('Internal server error')) {
                throw new Error('Server error occurred. Please try again or contact support if the problem persists.');
            } else {
                throw new Error(errorMsg);
            }
        }
        
    } catch (error) {
        console.error('Error uploading group photo:', error);
        
        // Provide more specific error messages
        let errorMessage = 'Error uploading group photo';
        if (error.message.includes('Network error')) {
            errorMessage = 'Network error. Please check your connection and try again.';
        } else if (error.message.includes('Invalid image format')) {
            errorMessage = 'Invalid image format. Please try a different image.';
        } else if (error.message.includes('Failed to process')) {
            errorMessage = 'Failed to process image. Please try a different image file.';
        } else if (error.message) {
            errorMessage += ': ' + error.message;
        }
        
        showError(errorMessage);
        
    } finally {
        // Reset button state
        if (uploadButton) {
            uploadButton.disabled = false;
            uploadButton.innerHTML = '<i class="upload icon"></i> Change Photo';
        }
    }
}

function confirmLeaveGroup() {
    if (!currentGroupId) return;
    
    if (!confirm('Are you sure you want to leave this group? This action cannot be undone.')) {
        return;
    }
    
    leaveGroup();
}

async function leaveGroup() {
    if (!currentGroupId) return;
    
    try {
        const response = await leaveGroupAPI(currentGroupId);
        
        if (response.success) {
            showSuccess('Left group successfully!');
            $('#modalGroupDetails').modal('hide');
            
            // Refresh groups list
            setTimeout(() => {
                loadGroups();
            }, 1000);
        } else {
            throw new Error(response.error || 'Unknown error');
        }
        
    } catch (error) {
        console.error('Error leaving group:', error);
        showError('Error leaving group: ' + error.message);
    }
}

// Test function to create a minimal valid JPEG for testing
function createTestJPEG() {
    // Create a minimal 1x1 pixel JPEG image
    const canvas = document.createElement('canvas');
    canvas.width = 1;
    canvas.height = 1;
    const ctx = canvas.getContext('2d');
    
    // Fill with white
    ctx.fillStyle = '#FFFFFF';
    ctx.fillRect(0, 0, 1, 1);
    
    // Convert to JPEG
    const dataUrl = canvas.toDataURL('image/jpeg', 0.8);
    return dataUrl.split(',')[1]; // Return just the base64 part
}

// Utility function to convert file to base64 (preserves original format when possible, converts to JPEG for compatibility)
function fileToBase64(file) {
    return new Promise((resolve, reject) => {

        // Create a canvas to convert any image format to JPEG
        const canvas = document.createElement('canvas');
        const ctx = canvas.getContext('2d');
        const img = new Image();
        
        img.onload = function() {
            try {

                // Calculate new dimensions (max 640x640 for better quality while keeping size reasonable)
                const maxSize = 640;
                let { width, height } = img;
                
                // Ensure minimum size for very small images
                const minSize = 64;
                if (width < minSize && height < minSize) {
                    const scale = minSize / Math.max(width, height);
                    width *= scale;
                    height *= scale;
                }
                
                if (width > maxSize || height > maxSize) {
                    if (width > height) {
                        height = (height * maxSize) / width;
                        width = maxSize;
                    } else {
                        width = (width * maxSize) / height;
                        height = maxSize;
                    }
                }
                
                // Set canvas dimensions to calculated dimensions
                canvas.width = Math.round(width);
                canvas.height = Math.round(height);
                
                // WhatsApp only accepts JPEG format for group photos
                // Always convert to JPEG with white background
                const outputFormat = 'image/jpeg';
                const quality = 0.8;
                
                // Fill with white background (required for JPEG, removes transparency)
                ctx.fillStyle = '#FFFFFF';
                ctx.fillRect(0, 0, canvas.width, canvas.height);
                
                // Draw image on canvas with new dimensions
                ctx.drawImage(img, 0, 0, canvas.width, canvas.height);
                
                // Convert to appropriate format
                const dataUrl = canvas.toDataURL(outputFormat, quality);
                
                // Validate the data URL
                if (!dataUrl || !dataUrl.startsWith(`data:${outputFormat};base64,`)) {
                    throw new Error(`Failed to convert image to ${outputFormat} format`);
                }
                
                // Extract just the base64 part
                const base64 = dataUrl.split(',')[1];
                
                // Validate base64 string
                if (!base64 || base64.length === 0) {
                    throw new Error('Generated base64 string is empty');
                }
                
                // Final validation - ensure it's a valid base64 string
                try {
                    const decodedData = atob(base64);
                    
                    // Additional validation - check minimum size
                    if (decodedData.length < 10) {
                        throw new Error('Generated image data is too small');
                    }
                    
                    // Format-specific validation
                    if (outputFormat === 'image/jpeg') {
                        // Check JPEG magic bytes (FF D8 FF)
                        const firstBytes = decodedData.substring(0, 3);
                        const magicBytes = String.fromCharCode(0xFF, 0xD8, 0xFF);
                        if (firstBytes !== magicBytes) {
                            //console.warn('Warning: Generated data may not be a valid JPEG');
                        }
                    } else if (outputFormat === 'image/png') {
                        // Check PNG magic bytes (89 50 4E 47)
                        const firstBytes = decodedData.substring(0, 4);
                        const magicBytes = String.fromCharCode(0x89, 0x50, 0x4E, 0x47);
                        if (firstBytes !== magicBytes) {
                            //console.warn('Warning: Generated data may not be a valid PNG');
                        }
                    }
                    
                } catch (e) {
                    console.error('Base64 validation failed:', e);
                    throw new Error('Generated base64 string is invalid: ' + e.message);
                }
                
                resolve({
                    base64: base64,
                    format: outputFormat,
                    dataUrl: dataUrl
                });
                
            } catch (error) {
                console.error('Error processing image:', error);
                reject(error);
            }
        };
        
        img.onerror = function(error) {
            console.error('Failed to load image:', error);
            reject(new Error('Failed to load image file. Please ensure it is a valid image.'));
        };
        
        // Create object URL from file and load into image
        try {
            const objectUrl = URL.createObjectURL(file);
            
            // Set up cleanup
            const cleanup = () => {
                URL.revokeObjectURL(objectUrl);
            };
            
            // Store original handlers
            const originalOnload = img.onload;
            const originalOnerror = img.onerror;
            
            // Override handlers to include cleanup
            img.onload = function() {
                cleanup();
                originalOnload.call(this);
            };
            
            img.onerror = function(error) {
                cleanup();
                originalOnerror.call(this, error);
            };
            
            img.src = objectUrl;
            
        } catch (error) {
            console.error('Error creating object URL:', error);
            reject(new Error('Failed to process image file'));
        }
    });
}

// API Functions for Group Management
async function updateGroupName(groupJID, name) {
    const token = getLocalStorageItem('token');
    const myHeaders = new Headers();
    myHeaders.append('token', token);
    myHeaders.append('Content-Type', 'application/json');
    
    const response = await fetch(baseUrl + "/group/name", {
        method: "POST",
        headers: myHeaders,
        body: JSON.stringify({
            GroupJID: groupJID,
            Name: name
        })
    });
    
    const data = await response.json();
    return {
        success: data.code === 200,
        data: data.data,
        error: data.error || data.message
    };
}

async function updateGroupTopic(groupJID, topic) {
    const token = getLocalStorageItem('token');
    const myHeaders = new Headers();
    myHeaders.append('token', token);
    myHeaders.append('Content-Type', 'application/json');
    
    const response = await fetch(baseUrl + "/group/topic", {
        method: "POST",
        headers: myHeaders,
        body: JSON.stringify({
            GroupJID: groupJID,
            Topic: topic
        })
    });
    
    const data = await response.json();
    return {
        success: data.code === 200,
        data: data.data,
        error: data.error || data.message
    };
}

async function updateGroupAnnounce(groupJID, announce) {
    const token = getLocalStorageItem('token');
    const myHeaders = new Headers();
    myHeaders.append('token', token);
    myHeaders.append('Content-Type', 'application/json');
    
    const response = await fetch(baseUrl + "/group/announce", {
        method: "POST",
        headers: myHeaders,
        body: JSON.stringify({
            GroupJID: groupJID,
            Announce: announce
        })
    });
    
    const data = await response.json();
    return {
        success: data.code === 200,
        data: data.data,
        error: data.error || data.message
    };
}

async function updateGroupLocked(groupjid, locked) {
    const token = getLocalStorageItem('token');
    const myHeaders = new Headers();
    myHeaders.append('token', token);
    myHeaders.append('Content-Type', 'application/json');
    
    const response = await fetch(baseUrl + "/group/locked", {
        method: "POST",
        headers: myHeaders,
        body: JSON.stringify({
            GroupJID: groupjid,
            Locked: locked
        })
    });
    
    const data = await response.json();
    return {
        success: data.code === 200,
        data: data.data,
        error: data.error || data.message
    };
}

async function updateGroupEphemeral(groupjid, duration) {
    const token = getLocalStorageItem('token');
    const myHeaders = new Headers();
    myHeaders.append('token', token);
    myHeaders.append('Content-Type', 'application/json');
    
    const response = await fetch(baseUrl + "/group/ephemeral", {
        method: "POST",
        headers: myHeaders,
        body: JSON.stringify({
            GroupJID: groupjid,
            Duration: duration
        })
    });
    
    const data = await response.json();
    return {
        success: data.code === 200,
        data: data.data,
        error: data.error || data.message
    };
}

async function updateGroupParticipants(groupJID, action, participants) {
    const token = getLocalStorageItem('token');
    const myHeaders = new Headers();
    myHeaders.append('token', token);
    myHeaders.append('Content-Type', 'application/json');
    
    const response = await fetch(baseUrl + "/group/updateparticipants", {
        method: "POST",
        headers: myHeaders,
        body: JSON.stringify({
            GroupJID: groupJID,
            Action: action,
            Phone: participants
        })
    });
    
    const data = await response.json();
    return {
        success: data.code === 200,
        data: data.data,
        error: data.error || data.message
    };
}

async function getGroupInviteLink(groupJID, reset = false) {
    const token = getLocalStorageItem('token');
    const myHeaders = new Headers();
    myHeaders.append('token', token);
    myHeaders.append('Content-Type', 'application/json');
    
    const params = new URLSearchParams({
        groupJID: groupJID
    });
    
    if (reset) {
        params.append('reset', 'true');
    }
    
    const response = await fetch(baseUrl + "/group/invitelink?" + params.toString(), {
        method: "GET",
        headers: myHeaders
    });
    
    const data = await response.json();
    return {
        success: data.code === 200,
        data: data.data,
        error: data.error || data.message
    };
}

async function updateGroupPhoto(groupJID, photoBase64) {
    const token = getLocalStorageItem('token');
    const myHeaders = new Headers();
    myHeaders.append('token', token);
    myHeaders.append('Content-Type', 'application/json');
    
    // Ensure the image data is properly formatted for the backend
    // WhatsApp only accepts JPEG format for group photos
    const imageData = `data:image/jpeg;base64,${photoBase64}`;

    // Validate the image data format before sending
    if (!imageData.startsWith('data:image/')) {
        throw new Error('Invalid image format. Expected data URL format.');
    }
    
    // Ensure the payload matches exactly what the backend expects
    const payload = {
        GroupJID: groupJID,
        Image: imageData
    };
        
    try {
        const response = await fetch(baseUrl + "/group/photo", {
            method: "POST",
            headers: myHeaders,
            body: JSON.stringify(payload)
        });

        // Check if response is JSON
        const contentType = response.headers.get('content-type');
        
        if (!contentType || !contentType.includes('application/json')) {
            throw new Error(`Server returned non-JSON response (${response.status}): ${response.statusText}`);
        }
        
        const data = await response.json();
        
        // Handle different response formats
        if (response.status === 200 && data.code === 200) {
            return {
                success: true,
                data: data.data,
                error: null
            };
        } else {
            // Handle error responses - provide detailed error information
            let errorMessage = 'Unknown server error';
            
            if (data.error) {
                errorMessage = data.error;
            } else if (data.message) {
                errorMessage = data.message;
            } else if (response.status === 500) {
                errorMessage = 'Internal server error. Please check server logs for details.';
            } else if (response.status === 400) {
                errorMessage = 'Bad request. Please check the image format and try again.';
            } else {
                errorMessage = `Server error (${response.status}): ${response.statusText}`;
            }
            
            console.error('Server error details:', {
                status: response.status,
                statusText: response.statusText,
                data: data,
                errorMessage: errorMessage
            });
            
            return {
                success: false,
                data: null,
                error: errorMessage
            };
        }
        
    } catch (error) {
        console.error('Network or parsing error:', error);
        return {
            success: false,
            data: null,
            error: `Network error: ${error.message}`
        };
    }
}

async function removeGroupPhotoAPI(groupjid) {
    const token = getLocalStorageItem('token');
    const myHeaders = new Headers();
    myHeaders.append('token', token);
    myHeaders.append('Content-Type', 'application/json');
    
    const response = await fetch(baseUrl + "/group/photo/remove", {
        method: "POST",
        headers: myHeaders,
        body: JSON.stringify({
            GroupJID: groupjid
        })
    });
    
    const data = await response.json();
    return {
        success: data.code === 200,
        data: data.data,
        error: data.error || data.message
    };
}

async function leaveGroupAPI(groupJID) {
    const token = getLocalStorageItem('token');
    const myHeaders = new Headers();
    myHeaders.append('token', token);
    myHeaders.append('Content-Type', 'application/json');
    
    const response = await fetch(baseUrl + "/group/leave", {
        method: "POST",
        headers: myHeaders,
        body: JSON.stringify({
            GroupJID: groupJID
        })
    });
    
    const data = await response.json();
    return {
        success: data.code === 200,
        data: data.data,
        error: data.error || data.message
    };
} 