# language: en

@labels @webhook @whatsappbusiness
Feature: Managing WhatsApp labels
  As a WuzAPI operator
  I want to manage WhatsApp labels through WuzAPI
  So that I can organize conversations and react to label changes

  Background:
    Given a linked WuzAPI instance named "Wuzapi"
    And the instance is subscribed to label events

  Scenario: Create a label and receive a label edit event
    When I create a label named "Leads" with color 5
    Then the label operation succeeds
    And a "LabelEdit" event is received for label "Leads"
    And the event says the label color is 5
    And the event says the label is not deleted
    And the label "Leads" appears in the label list

  Scenario: Rename a label and receive a label edit event
    Given a label named "Leads" exists
    When I rename that label to "Hot Leads"
    Then the label operation succeeds
    And a "LabelEdit" event is received for label "Hot Leads"
    And the label "Hot Leads" appears in the label list

  Scenario: Delete a label and receive a label edit event
    Given a label named "Leads" exists
    When I delete that label
    Then the label operation succeeds
    And a "LabelEdit" event is received for that label
    And the event says the label is deleted
    And that label appears as deleted in the label list

  Scenario: Apply and remove a label from a chat
    Given a label named "Leads" exists
    When I apply that label to the test contact chat
    Then a "LabelAssociationChat" event is received for that chat and label
    And the event says the chat is labeled
    And the test contact chat appears under label "Leads" in the label list
    When I remove that label from the test contact chat
    Then a "LabelAssociationChat" event is received for that chat and label
    And the event says the chat is not labeled
    And the test contact chat does not appear under label "Leads" in the label list

  Scenario: List labels
    Given a label named "Leads" exists
    And a label named "Follow Up" exists
    When I list labels
    Then the label list contains "Leads"
    And the label list contains "Follow Up"
    And the label list does not contain deleted labels

  Scenario: List labels with chat associations
    Given a label named "Leads" exists
    And the test contact chat has the label "Leads"
    When I list labels
    Then the chat label list contains label "Leads"
    And the test contact chat appears under label "Leads"
