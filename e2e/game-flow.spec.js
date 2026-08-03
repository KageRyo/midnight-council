const { test, expect } = require("@playwright/test");

test("four players can complete a private-role game and reset for a rematch", async ({ browser }) => {
  const roomID = `e2e-${Date.now()}`;
  const participants = [];
  for (const name of ["Owner", "Player Two", "Player Three", "Player Four"]) {
    const context = await browser.newContext();
    const page = await context.newPage();
    await page.goto(`/?room=${roomID}`);
    await page.locator("#player-name").fill(name);
    await page.locator("#join-button").click();
    await expect(page.locator("#game-view")).toBeVisible();
    participants.push({ context, page, name });
  }

  try {
    const owner = participants[0];
    await owner.page.locator("#game-preset-select").selectOption("MINIMAL");
    await owner.page.locator("#game-preset-select").selectOption("CUSTOM");
    for (const selector of [
      "#setting-night-seconds",
      "#setting-discussion-seconds",
      "#setting-voting-seconds",
      "#setting-last-words-seconds",
    ]) {
      await owner.page.locator(selector).fill("1");
    }
    await owner.page.locator("#setting-minimum-players").fill("4");
    await owner.page.locator("#game-settings-submit").click();
    await expect(owner.page.locator("#game-settings-preset-badge")).toHaveText("自訂");

    for (const participant of participants.slice(1)) {
      await participant.page.locator("#ready-button").click();
    }
    await expect(owner.page.locator("#start-game-button")).toBeEnabled();
    await owner.page.locator("#start-game-button").click();

    for (const participant of participants) {
      await expect(participant.page.locator("#phase-label")).toHaveText("夜晚");
      await expect(participant.page.locator("#role-name")).not.toHaveText("尚未揭曉");
      await expect(participant.page.locator("#player-list")).not.toContainText(/殺手|平民/);
    }

    let killerParticipant;
    for (const participant of participants) {
      if ((await participant.page.locator("#role-name").textContent()) === "殺手") {
        killerParticipant = participant;
        break;
      }
    }
    expect(killerParticipant, "one participant should receive the killer role").toBeTruthy();

    await killerParticipant.page.locator("#night-target").selectOption({ index: 0 });
    await killerParticipant.page.locator("#night-action-button").click();
    for (const participant of participants) {
      await expect(participant.page.locator("#phase-label")).toHaveText("白天討論");
    }

    const reconnectingParticipant = participants.find((participant) => participant !== killerParticipant);
    await reconnectingParticipant.page.reload();
    await reconnectingParticipant.page.locator("#join-button").click();
    await expect(reconnectingParticipant.page.locator("#connection-label")).toHaveText("已連線");
    await expect(reconnectingParticipant.page.locator("#role-name")).not.toHaveText("尚未揭曉");

    await owner.page.locator("#start-vote-button").click();
    for (const participant of participants) {
      await expect(participant.page.locator("#phase-label")).toHaveText("白天表決");
    }

    for (const participant of participants) {
      if (!(await participant.page.locator("#vote-form").isVisible())) {
        continue;
      }
      const options = await participant.page.locator("#vote-target option").allTextContents();
      const target = participant === killerParticipant
        ? options.find((option) => option && option !== killerParticipant.name)
        : killerParticipant.name;
      await participant.page.locator("#vote-target").selectOption({ label: target });
      await participant.page.locator("#vote-form button").click();
    }

    await expect(owner.page.locator("#phase-label")).toHaveText("遺言");
    await expect(owner.page.locator("#phase-label")).toHaveText("已結束");
    owner.page.on("dialog", (dialog) => dialog.accept());
    await owner.page.locator("#return-waiting-button").click();
    await expect(owner.page.locator("#phase-label")).toHaveText("等待中");
  } finally {
    await Promise.all(participants.map(({ context }) => context.close()));
  }
});
