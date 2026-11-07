import XCTest

final class ClientUITests: XCTestCase {
    private var app: XCUIApplication!
    override func setUpWithError() throws {
        continueAfterFailure=false
        XCUIDevice.shared.orientation = .portrait
        let url=URL(string:"http://127.0.0.1:18768/_fixture/reset")!
        var request=URLRequest(url:url);request.httpMethod="POST"
        let done=expectation(description:"Reset owned fixture")
        URLSession.shared.dataTask(with:request) { _,response,error in
            XCTAssertNil(error);XCTAssertEqual((response as? HTTPURLResponse)?.statusCode,200);done.fulfill()
        }.resume()
        wait(for:[done],timeout:10)
        app=XCUIApplication();app.launchArguments=["--uitesting-reset"]
        app.launchEnvironment["APTEX_UI_TEST_SERVER"]="http://127.0.0.1:18768"
        app.launch()
    }
    override func tearDownWithError() throws {
        if let app,(testRun?.failureCount ?? 0)>0 {
            let image=XCTAttachment(screenshot:app.screenshot());image.name="APTEX UI failure";image.lifetime = .keepAlways;add(image)
        }
    }
    private func login() {
        let password=app.secureTextFields["password"]
        XCTAssertTrue(password.waitForExistence(timeout:10));password.tap()
        // Match Odysseus's per-character events: iPad focus transitions can
        // otherwise drop part of a single injected secure-field string.
        for character in "fixture-aptex-1234" {password.typeText(String(character))}
        app.buttons["connect"].tap()
        XCTAssertTrue(app.staticTexts["Local fixture assessment"].waitForExistence(timeout:15))
    }
    private func tab(_ index:Int)->XCUIElement {
        let compact=app.tabBars.buttons.element(boundBy:index)
        if compact.exists {return compact}
        // Modern iPadOS exposes its floating tab items as cells/buttons rather
        // than a TabBar. Match the visible localized label on those controls.
        let labels=[["Tasks","Tareas"],["Chat"],["Findings","Hallazgos"],["Console","Consola"],["Settings","Ajustes"]]
        return app.descendants(matching:.any).matching(NSPredicate(format:"label IN %@ AND (elementType == %d OR elementType == %d)",labels[index],XCUIElement.ElementType.button.rawValue,XCUIElement.ElementType.cell.rawValue)).firstMatch
    }
    func testLoginTaskPauseFindingsAndLogout() {
        login()
        app.staticTexts["Local fixture assessment"].tap()
        let pause=app.buttons["Pause task"];XCTAssertTrue(pause.waitForExistence(timeout:10));pause.tap()
        XCTAssertTrue(app.buttons["Resume task"].waitForExistence(timeout:10))
        tab(2).tap()
        XCTAssertTrue(app.staticTexts["Fixture finding"].waitForExistence(timeout:10));app.staticTexts["Fixture finding"].tap()
        XCTAssertTrue(app.staticTexts["Fixture evidence retained."].waitForExistence(timeout:10))
        tab(4).tap()
        app.buttons["sign-out"].tap()
        XCTAssertTrue(app.secureTextFields["password"].waitForExistence(timeout:10))
        app.terminate();app.launchArguments=[];app.launch()
        XCTAssertTrue(app.secureTextFields["password"].waitForExistence(timeout:10))
    }
    func testChatDraftLiveSendAndConsoleCookieHandoff() {
        login();tab(1).tap()
        XCTAssertTrue(app.buttons["conversation-7"].waitForExistence(timeout:10));app.buttons["conversation-7"].tap()
        let ready=app.staticTexts.matching(NSPredicate(format:"label CONTAINS %@","Full message detail is loaded automatically.")).firstMatch.waitForExistence(timeout:10)
        if !ready {let image=XCTAttachment(screenshot:app.screenshot());image.name="Chat detail failure";image.lifetime = .keepAlways;add(image);print(app.debugDescription)}
        XCTAssertTrue(ready)
        var rewrite=URLRequest(url:URL(string:"http://127.0.0.1:18768/_fixture/rewrite")!);rewrite.httpMethod="POST"
        let rewritten=expectation(description:"Rewrite owned fixture history")
        URLSession.shared.dataTask(with:rewrite) {_,response,error in
            XCTAssertNil(error);XCTAssertEqual((response as? HTTPURLResponse)?.statusCode,200);rewritten.fulfill()
        }.resume();wait(for:[rewritten],timeout:10)
        XCTAssertTrue(app.staticTexts["Updated full message after history revision."].waitForExistence(timeout:10))
        let draft=app.textFields["message-draft"]
        XCTAssertTrue(draft.waitForExistence(timeout:10));draft.tap();draft.typeText("Native fixture message")
        app.buttons["send-message"].tap()
        XCTAssertTrue(app.staticTexts["Fixture reply: Native fixture message"].waitForExistence(timeout:10))
        draft.tap();draft.typeText("unsent draft survives tabs")
        app.buttons["dismiss-keyboard"].tap()
        XCTAssertTrue(tab(3).isHittable)
        tab(3).tap()
        XCTAssertTrue(app.webViews.staticTexts["Console fixture ready"].waitForExistence(timeout:15))
        tab(1).tap()
        XCTAssertEqual(app.textFields["message-draft"].value as? String,"unsent draft survives tabs")
        let shot=XCTAttachment(screenshot:app.screenshot());shot.name="APTEX native chat";shot.lifetime = .keepAlways;add(shot)
    }
    func testSavedLoginRestoresAndThemesRemainIndependent() {
        login();tab(4).tap()
        let theme=app.buttons["theme-picker"]
        XCTAssertTrue(theme.waitForExistence(timeout:10));theme.tap();app.buttons["Phosphor"].tap()
        app.terminate();app.launchArguments=[];app.launch()
        XCTAssertTrue(app.staticTexts["Local fixture assessment"].waitForExistence(timeout:15))
        tab(4).tap()
        XCTAssertTrue(app.buttons["theme-picker"].label.contains("Phosphor"))
        let about=app.staticTexts["APTEX by Afterglow Labs"]
        if !about.isHittable {app.swipeUp()}
        XCTAssertTrue(about.waitForExistence(timeout:5))
        let shot=XCTAttachment(screenshot:app.screenshot());shot.name="APTEX native settings";shot.lifetime = .keepAlways;add(shot)
    }
    func testLanguageSwitchPreservesAnUnsentDraft() {
        login();tab(1).tap()
        app.buttons["conversation-7"].tap()
        let draft=app.textFields["message-draft"]
        XCTAssertTrue(draft.waitForExistence(timeout:10));draft.tap();draft.typeText("draft across languages")
        app.buttons["dismiss-keyboard"].tap()
        tab(4).tap()
        app.buttons["language-picker"].tap();app.buttons["Español"].tap()
        XCTAssertTrue(app.navigationBars["Ajustes"].waitForExistence(timeout:10))
        tab(1).tap()
        XCTAssertEqual(app.textFields["message-draft"].value as? String,"draft across languages")
        tab(4).tap()
        app.buttons["language-picker"].tap();app.buttons["English"].tap()
        XCTAssertTrue(app.navigationBars["Settings"].waitForExistence(timeout:10))
    }

    func testConsoleHTTPExportOpensTheShareSheet() {
        login();tab(3).tap()
        XCTAssertTrue(app.webViews.staticTexts["Console fixture ready"].waitForExistence(timeout:15))
        app.webViews.links["Download fixture report"].tap()
        let presented=NSPredicate { _,_ in self.app.otherElements["export-share-sheet"].exists || self.app.navigationBars["UIActivityContentView"].exists }
        expectation(for:presented,evaluatedWith:nil);waitForExpectations(timeout:10)
        let image=XCTAttachment(screenshot:app.screenshot());image.name="APTEX export share sheet";image.lifetime = .keepAlways;add(image)
    }
    func testConsoleBlobExportOpensTheShareSheet() {
        login();tab(3).tap()
        XCTAssertTrue(app.webViews.staticTexts["Console fixture ready"].waitForExistence(timeout:15))
        app.webViews.buttons["Export fixture blob"].tap()
        let presented=NSPredicate { _,_ in self.app.otherElements["export-share-sheet"].exists || self.app.navigationBars["UIActivityContentView"].exists }
        expectation(for:presented,evaluatedWith:nil);waitForExpectations(timeout:10)
        let image=XCTAttachment(screenshot:app.screenshot());image.name="APTEX export share sheet";image.lifetime = .keepAlways;add(image)
    }

    func testNativeModelSelectionAndAttachmentChoices() {
        login();tab(1).tap()
        XCTAssertTrue(app.buttons["conversation-7"].waitForExistence(timeout:10));app.buttons["conversation-7"].tap()
        XCTAssertTrue(app.buttons["model-picker"].waitForExistence(timeout:10));app.buttons["model-picker"].tap()
        app.buttons["profile-selection"].tap()
        app.buttons["SOL fixture · fixture-sol · Blue"].tap()
        app.buttons["Apply"].tap()
        XCTAssertTrue(app.buttons["model-picker"].label.contains("SOL fixture"))
        app.buttons["Attach files"].tap()
        XCTAssertTrue(app.buttons["Choose photos"].waitForExistence(timeout:5))
        XCTAssertTrue(app.buttons["Choose files"].exists)
        app.buttons["Choose files"].tap()
        let cancel=app.buttons["Cancel"].firstMatch
        XCTAssertTrue(cancel.waitForExistence(timeout:10));cancel.tap()
        XCTAssertTrue(app.textFields["message-draft"].waitForExistence(timeout:10))
    }


}
