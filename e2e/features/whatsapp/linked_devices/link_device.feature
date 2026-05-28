# language: en

Feature: Linking WuzAPI to WhatsApp
  As a WuzAPI operator
  I want to link WuzAPI to WhatsApp using a pairing code
  So that WuzAPI can send and receive messages for me

  Scenario: Link WuzAPI with a pairing code
    Given a WuzAPI instance named "Wuzapi"
    And WhatsApp is open on the phone
    And the linked devices screen is open
    And no device named "Wuzapi" is currently linked
    When I choose to link a new device using a phone number
    And WuzAPI requests a pairing code for "Wuzapi"
    And I enter that pairing code in WhatsApp
    And I name the linked device "Wuzapi"
    Then "Wuzapi" appears in the list of linked devices
