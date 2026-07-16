(() => {
  "use strict";

  const SESSION_KEY = "midnight-council.sessions.v1";
  const LAST_NAME_KEY = "midnight-council.last-name.v1";
  const MAX_LOCAL_CHAT_MESSAGES = 150;

  const roles = {
    VILLAGER: {
      name: "平民",
      symbol: "◇",
      description: "觀察言行、參與表決，與議會一同找出殺手。",
    },
    KILLER: {
      name: "殺手",
      symbol: "◆",
      description: "每晚選擇一名目標，讓殺手陣營控制議會。",
    },
    DETECTIVE: {
      name: "偵探",
      symbol: "◉",
      description: "每晚查驗一名玩家，結果只有你能看見。",
    },
    DOCTOR: {
      name: "醫生",
      symbol: "✚",
      description: "每晚保護一名玩家，也可以選擇保護自己。",
    },
    SHOOTER: {
      name: "槍手",
      symbol: "✦",
      description: "白天可開槍一次，使一名存活玩家立即出局。",
    },
  };

  const phases = {
    WAITING: {
      label: "等待中",
      kicker: "ASSEMBLING",
      title: "等待議員就座",
      description: "所有非房主玩家準備完成後，房主即可開始遊戲。",
      symbol: "◐",
    },
    NIGHT: {
      label: "夜晚",
      kicker: "NIGHT PHASE",
      title: "天黑請閉眼",
      description: "擁有夜間能力的玩家秘密行動；所有行動提交後將立即結算。",
      symbol: "◑",
    },
    DAY_DISCUSSION: {
      label: "白天討論",
      kicker: "OPEN DISCUSSION",
      title: "晨光下的辯論",
      description: "檢視昨夜結果、交換線索。房主可提早表決，倒數結束也會自動進入投票。",
      symbol: "☼",
    },
    DAY_VOTING: {
      label: "白天表決",
      kicker: "COUNCIL VOTE",
      title: "做出你的選擇",
      description: "所有存活玩家都必須投票或棄權；倒數結束時，尚未投票者會自動棄權。",
      symbol: "⌁",
    },
    FINISHED: {
      label: "已結束",
      kicker: "FINAL VERDICT",
      title: "議會已作出裁決",
      description: "本局所有身分已公開。房主可返回等待室，在同一房間開始下一局。",
      symbol: "✦",
    },
  };

  const logLabels = {
    game_started: "遊戲開始",
    night_started: "夜幕降臨",
    day_started: "天亮了",
    night_eliminated: "昨夜有人出局",
    night_no_elimination: "昨夜無人出局",
    voting_started: "表決開始",
    player_executed: "議會執行處決",
    vote_no_execution: "本輪無人被處決",
    shooter_fired: "槍手開火",
    phase_timed_out: "階段時間結束",
    game_finished: "遊戲結束",
  };

  const app = {
    socket: null,
    roomID: "",
    playerID: "",
    playerName: "",
    spectator: false,
    state: null,
    private: null,
    chats: [],
    intentionallyClosed: false,
    toastTimer: null,
    countdownInterval: null,
    serverClockOffset: 0,
  };

  const ui = Object.fromEntries(
    [
      "connection-status",
      "connection-label",
      "join-view",
      "join-form",
      "room-id",
      "player-name",
      "join-as-spectator",
      "random-room-button",
      "resume-note",
      "join-error",
      "join-button",
      "game-view",
      "room-code",
      "copy-room-button",
      "countdown-meta",
      "phase-countdown",
      "phase-label",
      "round-label",
      "room-access",
      "room-capacity",
      "leave-button",
      "life-badge",
      "role-sigil",
      "role-name",
      "role-description",
      "investigation-block",
      "investigation-list",
      "player-count",
      "player-list",
      "spectators-card",
      "spectator-count",
      "spectator-list",
      "room-admin-card",
      "room-lock-button",
      "player-limit-form",
      "player-limit-input",
      "owner-transfer-form",
      "owner-transfer-target",
      "return-waiting-button",
      "room-admin-hint",
      "phase-art",
      "phase-kicker",
      "phase-title",
      "phase-description",
      "result-panel",
      "result-icon",
      "result-title",
      "result-reason",
      "waiting-actions",
      "ready-button",
      "start-game-button",
      "waiting-hint",
      "night-action-form",
      "night-target-label",
      "night-target",
      "night-action-button",
      "night-pass-button",
      "night-action-hint",
      "discussion-actions",
      "start-vote-button",
      "discussion-hint",
      "vote-form",
      "vote-target",
      "vote-hint",
      "shoot-form",
      "shoot-target",
      "chat-list",
      "chat-empty",
      "chat-form",
      "chat-message",
      "chat-hint",
      "event-log",
      "toast",
    ].map((id) => [toCamelCase(id), document.getElementById(id)]),
  );

  function toCamelCase(value) {
    return value.replace(/-([a-z])/g, (_, letter) => letter.toUpperCase());
  }

  function start() {
    const params = new URLSearchParams(window.location.search);
    const roomFromURL = params.get("room") || "";
    const lastName = safeLocalStorageGet(LAST_NAME_KEY) || "";

    ui.roomId.value = roomFromURL;
    ui.playerName.value = lastName;
    if (!ui.roomId.value) {
      ui.roomId.value = randomRoomID();
    }
    updateResumeNote();

    ui.joinForm.addEventListener("submit", joinRoom);
    ui.roomId.addEventListener("input", updateResumeNote);
    ui.randomRoomButton.addEventListener("click", () => {
      ui.roomId.value = randomRoomID();
      updateResumeNote();
      ui.playerName.focus();
    });
    ui.copyRoomButton.addEventListener("click", copyInviteLink);
    ui.leaveButton.addEventListener("click", leaveRoom);
    ui.readyButton.addEventListener("click", toggleReady);
    ui.startGameButton.addEventListener("click", () => sendEvent({ type: "start_game" }));
    ui.roomLockButton.addEventListener("click", toggleRoomLock);
    ui.playerLimitForm.addEventListener("submit", updatePlayerLimit);
    ui.ownerTransferForm.addEventListener("submit", transferOwner);
    ui.returnWaitingButton.addEventListener("click", returnToWaiting);
    ui.nightActionForm.addEventListener("submit", submitNightAction);
    ui.nightPassButton.addEventListener("click", () => sendEvent({ type: "night_pass" }));
    ui.startVoteButton.addEventListener("click", () => sendEvent({ type: "start_vote" }));
    ui.voteForm.addEventListener("submit", submitVote);
    ui.shootForm.addEventListener("submit", submitShot);
    ui.chatForm.addEventListener("submit", submitChat);
    window.addEventListener("beforeunload", () => {
      app.intentionallyClosed = true;
      app.socket?.close();
    });
  }

  function joinRoom(event) {
    event.preventDefault();
    hideJoinError();

    const roomID = ui.roomId.value.trim();
    const playerName = ui.playerName.value.trim();

    if (!/^[A-Za-z0-9_-]{2,48}$/.test(roomID)) {
      showJoinError("房間代號限用 2–48 個英文字母、數字、連字號或底線。");
      ui.roomId.focus();
      return;
    }
    if (!playerName || playerName.length > 32) {
      showJoinError("請輸入 1–32 個字元的議員名稱。");
      ui.playerName.focus();
      return;
    }

    const session = getSession(roomID);
    const spectator = session ? session.spectator === true : ui.joinAsSpectator.checked;
    app.roomID = roomID;
    app.playerID = session?.playerID || newPlayerID();
    app.playerName = playerName;
    app.spectator = spectator;
    app.state = null;
    app.private = null;
    app.chats = [];
    app.intentionallyClosed = false;

    safeLocalStorageSet(LAST_NAME_KEY, playerName);
    setConnection("connecting", "正在進入議會…");
    ui.joinButton.disabled = true;

    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const path = `/ws/rooms/${encodeURIComponent(roomID)}`;
    const query = new URLSearchParams({
      player_id: app.playerID,
      name: playerName,
    });
    if (session?.reconnectToken) {
      query.set("reconnect_token", session.reconnectToken);
    }
    if (spectator) {
      query.set("spectator", "true");
    }

    const socket = new WebSocket(`${protocol}//${window.location.host}${path}?${query}`);
    app.socket = socket;
    socket.addEventListener("open", () => {
      setConnection("online", "已連線");
    });
    socket.addEventListener("message", handleEnvelope);
    socket.addEventListener("error", () => {
      if (!app.state) {
        showJoinError("無法連接遊戲伺服器，請稍後再試。");
      }
    });
    socket.addEventListener("close", handleSocketClose);
  }

  function handleEnvelope(event) {
    let envelope;
    try {
      envelope = JSON.parse(event.data);
    } catch {
      showToast("收到無法解析的伺服器訊息。", true);
      return;
    }

    switch (envelope.type) {
      case "state":
        if (!envelope.state) {
          return;
        }
        syncServerClock(envelope.state.server_time);
        app.state = envelope.state;
        app.private = envelope.private || null;
        app.spectator = app.private?.spectator === true;
        if (app.private?.reconnect_token) {
          saveSession(app.roomID, {
            playerID: app.playerID,
            playerName: app.playerName,
            reconnectToken: app.private.reconnect_token,
            spectator: app.private.spectator === true,
          });
        }
        enterGameView();
        render();
        break;
      case "chat":
        if (envelope.chat) {
          app.chats.push(envelope.chat);
          app.chats = app.chats.slice(-MAX_LOCAL_CHAT_MESSAGES);
          renderChat();
        }
        break;
      case "error":
        handleServerError(envelope.error || "伺服器拒絕了這項操作。");
        break;
      default:
        showToast("收到不支援的伺服器訊息。", true);
    }
  }

  function handleServerError(message) {
    const translated = translateError(message);
    if (message.includes("removed from room by the owner")) {
      deleteSession(app.roomID);
      app.intentionallyClosed = true;
      app.socket?.close();
      app.socket = null;
      app.state = null;
      app.private = null;
      app.chats = [];
      stopCountdown();
      ui.gameView.hidden = true;
      ui.joinView.hidden = false;
      ui.roomId.value = app.roomID;
      setConnection("offline", "尚未連線");
      updateResumeNote();
      showJoinError(translated);
      return;
    }
    if (!app.state) {
      showJoinError(translated);
      if (message.includes("reconnect token") || message.includes("participant type")) {
        deleteSession(app.roomID);
        updateResumeNote();
      }
      return;
    }
    showToast(translated, true);
  }

  function handleSocketClose(event) {
    ui.joinButton.disabled = false;
    if (app.intentionallyClosed) {
      setConnection("offline", "尚未連線");
      return;
    }

    setConnection("offline", "連線中斷");
    stopCountdown(false);
    if (app.state) {
      showToast("與伺服器的連線已中斷。重新整理後可使用保存的憑證重連。", true);
      disableGameInputs();
    } else if (ui.joinError.hidden) {
      showJoinError(event.reason
        ? translateError(event.reason)
        : "連線已關閉，請確認房間資料後再試一次。");
    }
  }

  function enterGameView() {
    ui.joinView.hidden = true;
    ui.gameView.hidden = false;
    ui.joinButton.disabled = false;
    enableGameInputs();
    const url = new URL(window.location.href);
    url.search = new URLSearchParams({ room: app.roomID }).toString();
    window.history.replaceState(null, "", url);
  }

  function leaveRoom() {
    app.intentionallyClosed = true;
    app.socket?.close(1000, "player left");
    app.socket = null;
    app.state = null;
    app.private = null;
    app.chats = [];
    stopCountdown();
    ui.gameView.hidden = true;
    ui.joinView.hidden = false;
    ui.roomId.value = app.roomID;
    setConnection("offline", "尚未連線");
    updateResumeNote();
  }

  function sendEvent(payload) {
    if (!app.socket || app.socket.readyState !== WebSocket.OPEN) {
      showToast("目前未連線，無法送出操作。", true);
      return false;
    }
    app.socket.send(JSON.stringify(payload));
    return true;
  }

  function toggleReady() {
    const self = currentPlayer();
    if (self) {
      sendEvent({ type: "ready", ready: !self.ready });
    }
  }

  function toggleRoomLock() {
    sendEvent({ type: "set_room_locked", locked: !app.state.locked });
  }

  function updatePlayerLimit(event) {
    event.preventDefault();
    const maxPlayers = Number.parseInt(ui.playerLimitInput.value, 10);
    if (!Number.isInteger(maxPlayers) || maxPlayers < 2 || maxPlayers > 20) {
      showToast("玩家上限必須介於 2 到 20。", true);
      return;
    }
    sendEvent({ type: "set_player_limit", max_players: maxPlayers });
  }

  function transferOwner(event) {
    event.preventDefault();
    const targetID = ui.ownerTransferTarget.value;
    if (!targetID) {
      showToast("目前沒有可轉移房主的連線玩家。", true);
      return;
    }
    sendEvent({ type: "transfer_owner", target_id: targetID });
  }

  function returnToWaiting() {
    const active = !["WAITING", "FINISHED"].includes(app.state.phase);
    const prompt = active
      ? "確定要中止目前對局並返回等待室嗎？本局進度會被清除。"
      : "確定要返回等待室並準備下一局嗎？";
    if (window.confirm(prompt)) {
      sendEvent({ type: "return_to_waiting" });
    }
  }

  function kickParticipant(participantID, participantName) {
    if (window.confirm(`確定要將 ${participantName} 移出房間嗎？`)) {
      sendEvent({ type: "kick_participant", target_id: participantID });
    }
  }

  function submitNightAction(event) {
    event.preventDefault();
    if (!ui.nightTarget.value) {
      showToast("請先選擇夜間行動目標。", true);
      return;
    }
    sendEvent({ type: "night_action", target_id: ui.nightTarget.value });
  }

  function submitVote(event) {
    event.preventDefault();
    const payload = { type: "vote" };
    if (ui.voteTarget.value) {
      payload.target_id = ui.voteTarget.value;
    }
    sendEvent(payload);
  }

  function submitShot(event) {
    event.preventDefault();
    const targetID = ui.shootTarget.value;
    if (!targetID) {
      showToast("請先選擇射擊目標。", true);
      return;
    }
    const target = playerByID(targetID);
    if (!window.confirm(`確定要對 ${target?.name || "這名玩家"} 開槍嗎？這項操作無法撤銷。`)) {
      return;
    }
    sendEvent({ type: "shoot", target_id: targetID });
  }

  function submitChat(event) {
    event.preventDefault();
    const message = ui.chatMessage.value.trim();
    if (!message) {
      return;
    }
    if (sendEvent({ type: "chat", message })) {
      ui.chatMessage.value = "";
    }
  }

  function render() {
    if (!app.state) {
      return;
    }
    renderRoomHeader();
    renderIdentity();
    renderPlayers();
    renderSpectators();
    renderRoomAdmin();
    renderPhase();
    renderActions();
    renderChat();
    renderLog();
    startCountdown();
  }

  function renderRoomHeader() {
    const phase = phases[app.state.phase] || phases.WAITING;
    ui.roomCode.textContent = app.state.room_id;
    ui.phaseLabel.textContent = phase.label;
    ui.roundLabel.textContent = app.state.round > 0 ? String(app.state.round) : "—";
    ui.roomAccess.textContent = app.state.locked ? "已鎖定" : "公開";
    ui.roomCapacity.textContent = `${app.state.players?.length || 0} / ${app.state.max_players || 12}`;
  }

  function renderIdentity() {
    if (app.private?.spectator) {
      ui.lifeBadge.textContent = "旁觀";
      ui.lifeBadge.classList.remove("dead");
      ui.roleSigil.dataset.role = "SPECTATOR";
      ui.roleSigil.textContent = "◌";
      ui.roleName.textContent = "旁觀者";
      ui.roleDescription.textContent = "你不佔玩家席次，也不會取得角色或參與遊戲行動。";
      ui.investigationBlock.hidden = true;
      ui.investigationList.replaceChildren();
      return;
    }

    const role = roles[app.private?.role];
    const alive = app.private?.alive !== false;

    ui.lifeBadge.textContent = alive ? "存活" : "已出局";
    ui.lifeBadge.classList.toggle("dead", !alive);
    ui.roleSigil.dataset.role = app.private?.role || "UNKNOWN";
    ui.roleSigil.textContent = role?.symbol || "?";
    ui.roleName.textContent = role?.name || "尚未揭曉";
    ui.roleDescription.textContent = role?.description || "遊戲開始後，身分將只對你顯示。";

    const investigations = app.private?.investigations || [];
    ui.investigationBlock.hidden = investigations.length === 0;
    const items = investigations.map((result) => {
      const item = element("li");
      const name = element("span", "", `第 ${result.round} 輪 · ${result.target_name}`);
      const verdict = element("strong", result.killer ? "killer" : "safe", result.killer ? "殺手" : "非殺手");
      item.append(name, verdict);
      return item;
    });
    ui.investigationList.replaceChildren(...items);
  }

  function renderPlayers() {
    const players = app.state.players || [];
    const canKick = isCurrentOwner() && ["WAITING", "FINISHED"].includes(app.state.phase);
    ui.playerCount.textContent = String(players.length);

    const items = players.map((player) => {
      const item = element("li", "player-item");
      item.classList.toggle("self", player.id === app.playerID);
      item.classList.toggle("dead", !player.alive);

      const avatar = element("span", "player-avatar", initials(player.name));
      const copy = element("span", "player-copy");
      const name = element("strong", "", player.name + (player.id === app.playerID ? "（你）" : ""));
      const details = [];
      if (player.owner) details.push("房主");
      if (app.state.phase === "WAITING" && !player.owner) details.push(player.ready ? "已準備" : "未準備");
      if (app.state.phase !== "WAITING") details.push(player.alive ? "存活" : "已出局");
      if (player.role) details.push(roles[player.role]?.name || player.role);
      if (!player.connected) details.push("已斷線");
      copy.append(name, element("small", "", details.join(" · ")));

      const state = element("span", "player-state");
      state.classList.toggle("ready", app.state.phase === "WAITING" && (player.owner || player.ready));
      state.classList.toggle("connected", app.state.phase !== "WAITING" && player.connected);
      state.title = player.connected ? "已連線" : "已斷線";
      const actions = element("span", "participant-actions");
      actions.append(state);
      if (canKick && !player.owner) {
        const remove = element("button", "participant-remove-button", "×");
        remove.type = "button";
        remove.title = `移出 ${player.name}`;
        remove.setAttribute("aria-label", `將 ${player.name} 移出房間`);
        remove.addEventListener("click", () => kickParticipant(player.id, player.name));
        actions.append(remove);
      }
      item.append(avatar, copy, actions);
      return item;
    });

    ui.playerList.replaceChildren(...items);
  }

  function renderSpectators() {
    const spectators = app.state.spectators || [];
    const canKick = isCurrentOwner() && ["WAITING", "FINISHED"].includes(app.state.phase);
    ui.spectatorsCard.hidden = spectators.length === 0;
    ui.spectatorCount.textContent = String(spectators.length);

    const items = spectators.map((spectator) => {
      const item = element("li", "player-item");
      item.classList.toggle("self", spectator.id === app.playerID);
      const avatar = element("span", "player-avatar", initials(spectator.name));
      const copy = element("span", "player-copy");
      const name = element("strong", "", spectator.name + (spectator.id === app.playerID ? "（你）" : ""));
      copy.append(name, element("small", "", spectator.connected ? "旁觀者" : "旁觀者 · 已斷線"));

      const actions = element("span", "participant-actions");
      const state = element("span", "player-state");
      state.classList.toggle("connected", spectator.connected);
      state.title = spectator.connected ? "已連線" : "已斷線";
      actions.append(state);
      if (canKick) {
        const remove = element("button", "participant-remove-button", "×");
        remove.type = "button";
        remove.title = `移出 ${spectator.name}`;
        remove.setAttribute("aria-label", `將 ${spectator.name} 移出房間`);
        remove.addEventListener("click", () => kickParticipant(spectator.id, spectator.name));
        actions.append(remove);
      }
      item.append(avatar, copy, actions);
      return item;
    });

    ui.spectatorList.replaceChildren(...items);
  }

  function renderRoomAdmin() {
    const isOwner = isCurrentOwner();
    const phaseAllowsRosterChanges = ["WAITING", "FINISHED"].includes(app.state.phase);
    ui.roomAdminCard.hidden = !isOwner;
    if (!isOwner) {
      return;
    }

    ui.roomLockButton.textContent = app.state.locked ? "解除鎖房" : "鎖定房間";
    ui.playerLimitInput.value = String(app.state.max_players || 12);
    ui.playerLimitInput.disabled = !phaseAllowsRosterChanges;
    ui.playerLimitForm.querySelector("button").disabled = !phaseAllowsRosterChanges;

    const ownerTargets = (app.state.players || []).filter((player) => !player.owner && player.connected);
    ui.ownerTransferTarget.replaceChildren(
      option("", ownerTargets.length === 0 ? "沒有可轉移的玩家" : "選擇新房主"),
      ...ownerTargets.map((player) => option(player.id, player.name)),
    );
    ui.ownerTransferTarget.disabled = ownerTargets.length === 0;
    ui.ownerTransferForm.querySelector("button").disabled = ownerTargets.length === 0;

    ui.returnWaitingButton.hidden = app.state.phase === "WAITING";
    ui.returnWaitingButton.textContent = app.state.phase === "FINISHED" ? "返回等待室，再來一局" : "中止對局並返回等待室";
    ui.roomAdminHint.textContent = phaseAllowsRosterChanges
      ? "等待或結算階段可調整席次與移除參與者。"
      : "對局進行中仍可鎖房、轉移房主或返回等待室。";
  }

  function renderPhase() {
    const phase = phases[app.state.phase] || phases.WAITING;
    ui.phaseLabel.textContent = phase.label;
    ui.phaseKicker.textContent = app.state.round > 0 ? `ROUND ${app.state.round} · ${phase.kicker}` : phase.kicker;
    ui.phaseTitle.textContent = phase.title;
    ui.phaseDescription.textContent = phase.description;
    ui.phaseArt.dataset.phase = app.state.phase;
    ui.phaseArt.querySelector(".phase-symbol").textContent = phase.symbol;

    const result = app.state.result;
    ui.resultPanel.hidden = !result;
    if (result) {
      const villagersWon = result.winner === "VILLAGERS";
      ui.resultIcon.textContent = villagersWon ? "✦" : "◆";
      ui.resultTitle.textContent = villagersWon ? "議會陣營獲勝" : "殺手陣營獲勝";
      ui.resultReason.textContent = result.reason === "all_killers_eliminated"
        ? "所有殺手都已被淘汰。"
        : "殺手已控制半數以上的存活席次。";
    }
  }

  function renderActions() {
    ui.waitingActions.hidden = true;
    ui.nightActionForm.hidden = true;
    ui.discussionActions.hidden = true;
    ui.voteForm.hidden = true;
    ui.shootForm.hidden = true;

    const self = currentPlayer();
    const isOwner = isCurrentOwner();
    const phase = app.state.phase;

    if (app.private?.spectator) {
      ui.phaseDescription.textContent = ["WAITING", "FINISHED"].includes(phase)
        ? "你正以旁觀者身分留在房間，可閱讀公開資訊與參與公開聊天。"
        : "你正在旁觀本局；遊戲進行中無法執行動作或在公開頻道發言。";
      updateChatAvailability();
      return;
    }

    if (phase === "WAITING") {
      ui.waitingActions.hidden = false;
      ui.readyButton.hidden = isOwner;
      ui.startGameButton.hidden = !isOwner;
      if (isOwner) {
        const nonOwners = app.state.players.filter((player) => !player.owner);
        const enoughPlayers = app.state.players.length >= 2;
        const everyoneReady = nonOwners.every((player) => player.ready);
        ui.startGameButton.disabled = !enoughPlayers || !everyoneReady;
        ui.waitingHint.textContent = !enoughPlayers
          ? "至少需要兩名玩家。分享邀請連結，等待另一位議員加入。"
          : everyoneReady
            ? "所有人都已準備，可以開始遊戲。"
            : "等待所有非房主玩家完成準備。";
      } else if (self) {
        ui.readyButton.textContent = self.ready ? "取消準備" : "我已準備";
        ui.waitingHint.textContent = self.ready ? "已準備，等待房主開始遊戲。" : "準備完成後通知房主。";
      }
      updateChatAvailability();
      return;
    }

    if (phase === "NIGHT" && available("night_action")) {
      ui.nightActionForm.hidden = false;
      const role = app.private?.role;
      const action = {
        KILLER: ["選擇刺殺目標", "執行刺殺"],
        DETECTIVE: ["選擇查驗目標", "進行查驗"],
        DOCTOR: ["選擇保護目標", "進行保護"],
      }[role] || ["選擇夜間目標", "確認行動"];
      ui.nightTargetLabel.textContent = action[0];
      ui.nightActionButton.textContent = action[1];
      fillPlayerSelect(ui.nightTarget, (player) => {
        if (!player.alive) return false;
        return role === "DOCTOR" || player.id !== app.playerID;
      });
      ui.nightActionHint.textContent = app.private?.action_required
        ? "你的行動尚未提交；倒數結束時將自動跳過。"
        : "行動已提交；夜晚結算前仍可變更目標。";
    } else if (phase === "NIGHT" && app.private?.alive) {
      ui.phaseDescription.textContent = "你在今晚沒有指定行動。等待其他角色完成秘密行動。";
    } else if (phase === "NIGHT") {
      ui.phaseDescription.textContent = "你已出局，請等待存活玩家完成夜間行動。";
    }

    if (phase === "DAY_DISCUSSION") {
      ui.discussionActions.hidden = false;
      ui.startVoteButton.hidden = !isOwner;
      ui.discussionHint.textContent = isOwner
        ? "確認大家討論完畢後可提早表決，否則倒數結束會自動開始。"
        : "交換線索；房主提早開始或倒數結束後將進入表決。";
    }

    if (phase === "DAY_VOTING" && app.private?.can_vote) {
      ui.voteForm.hidden = false;
      fillPlayerSelect(ui.voteTarget, (player) => player.alive, true);
      if (app.private.voted_for) {
        ui.voteTarget.value = app.private.voted_for;
        ui.voteHint.textContent = "表決已送出；結算前仍可改票。";
      } else {
        ui.voteHint.textContent = "也可以選擇棄權；倒數結束時尚未投票者會自動棄權。";
      }
    } else if (phase === "DAY_VOTING" && !app.private?.alive) {
      ui.phaseDescription.textContent = "你已出局，無法參與表決。等待存活玩家完成投票。";
    }

    if (app.private?.can_shoot && available("shoot")) {
      ui.shootForm.hidden = false;
      fillPlayerSelect(ui.shootTarget, (player) => player.alive && player.id !== app.playerID);
    }

    updateChatAvailability();
  }

  function renderChat() {
    const items = app.chats.map((message) => {
      const item = element("li", "chat-message");
      const avatar = element("span", "player-avatar", initials(message.name));
      const content = element("div");
      const header = document.createElement("header");
      const name = element("strong", "", message.name + (message.player_id === app.playerID ? "（你）" : ""));
      const time = element("time", "", formatTime(message.sent_at));
      time.dateTime = message.sent_at;
      header.append(name, time);
      content.append(header, element("p", "", message.message));
      item.append(avatar, content);
      return item;
    });

    if (items.length === 0) {
      ui.chatList.replaceChildren(element("li", "empty-message", "尚無發言。打破沉默，但別太早暴露自己。"));
      return;
    }
    ui.chatList.replaceChildren(...items);
    ui.chatList.scrollTop = ui.chatList.scrollHeight;
  }

  function renderLog() {
    const logs = app.state.log || [];
    if (logs.length === 0) {
      ui.eventLog.replaceChildren(element("li", "empty-message", "遊戲事件將記錄於此。"));
      return;
    }

    const items = logs.slice().reverse().map((entry) => {
      const item = element("li", "event-item");
      item.classList.toggle("important", ["game_started", "game_finished", "player_executed", "night_eliminated", "phase_timed_out"].includes(entry.type));
      const title = element("strong", "", logLabels[entry.type] || entry.type);
      const detail = element("span", "", describeLog(entry));
      item.append(title, detail);
      return item;
    });
    ui.eventLog.replaceChildren(...items);
  }

  function describeLog(entry) {
    const playerName = playerByID(entry.player_id)?.name;
    const targetName = playerByID(entry.target_id)?.name;
    switch (entry.type) {
      case "game_started":
        return `${playerName || "房主"} 開始了遊戲`;
      case "night_started":
        return `第 ${entry.round} 輪夜晚`;
      case "day_started":
        return `第 ${entry.round} 輪白天`;
      case "night_eliminated":
        return `${targetName || "一名玩家"} 在夜裡出局`;
      case "night_no_elimination":
        return "所有人都迎來了早晨";
      case "voting_started":
        return `${playerName || "房主"} 宣布開始表決`;
      case "player_executed":
        return `${targetName || "一名玩家"} 遭議會處決`;
      case "vote_no_execution":
        return "平票或無有效票數";
      case "shooter_fired":
        return `${playerName || "槍手"} 對 ${targetName || "一名玩家"} 開槍`;
      case "phase_timed_out":
        return `${phases[entry.phase]?.label || "目前階段"}倒數結束，伺服器已自動推進`;
      case "game_finished":
        return entry.winner === "VILLAGERS" ? "議會陣營獲勝" : "殺手陣營獲勝";
      default:
        return formatTime(entry.at);
    }
  }

  function updateChatAvailability() {
    const connected = app.socket?.readyState === WebSocket.OPEN;
    const ongoing = !["WAITING", "FINISHED"].includes(app.state.phase);
    const dead = app.private?.alive === false;
    const spectator = app.private?.spectator === true;
    const disabled = !connected || (ongoing && (dead || spectator));
    ui.chatMessage.disabled = disabled;
    ui.chatForm.querySelector("button").disabled = disabled;
    ui.chatHint.textContent = ongoing && spectator
      ? "旁觀者在遊戲結束前無法公開發言。"
      : ongoing && dead
        ? "出局玩家在遊戲結束前無法公開發言。"
        : "";
  }

  function disableGameInputs() {
    ui.gameView.querySelectorAll("button, input, select").forEach((control) => {
      if (control !== ui.leaveButton) {
        control.disabled = true;
      }
    });
  }

  function syncServerClock(serverTime) {
    const parsed = Date.parse(serverTime);
    if (Number.isFinite(parsed)) {
      app.serverClockOffset = parsed - Date.now();
    }
  }

  function startCountdown() {
    stopCountdown();
    const deadline = Date.parse(app.state?.phase_deadline);
    if (!Number.isFinite(deadline)) {
      return;
    }

    ui.countdownMeta.hidden = false;
    const update = () => {
      const remaining = Math.max(0, deadline - (Date.now() + app.serverClockOffset));
      ui.phaseCountdown.textContent = formatCountdown(remaining);
      switch (true) {
        case remaining <= 0:
          ui.countdownMeta.dataset.urgency = "expired";
          ui.countdownMeta.title = "時間已到，等待伺服器結算";
          window.clearInterval(app.countdownInterval);
          app.countdownInterval = null;
          break;
        case remaining <= 10_000:
          ui.countdownMeta.dataset.urgency = "urgent";
          ui.countdownMeta.title = "階段即將結束";
          break;
        default:
          ui.countdownMeta.dataset.urgency = "normal";
          ui.countdownMeta.title = "伺服器權威階段倒數";
      }
    };

    update();
    if (deadline > Date.now() + app.serverClockOffset) {
      app.countdownInterval = window.setInterval(update, 250);
    }
  }

  function stopCountdown(hide = true) {
    window.clearInterval(app.countdownInterval);
    app.countdownInterval = null;
    if (hide) {
      ui.countdownMeta.hidden = true;
    } else {
      ui.phaseCountdown.textContent = "--:--";
      ui.countdownMeta.dataset.urgency = "expired";
      ui.countdownMeta.title = "連線中斷，倒數已停止同步";
    }
  }

  function formatCountdown(milliseconds) {
    const totalSeconds = Math.ceil(milliseconds / 1000);
    const minutes = Math.floor(totalSeconds / 60);
    const seconds = totalSeconds % 60;
    return `${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`;
  }

  function enableGameInputs() {
    ui.gameView.querySelectorAll("button, input, select").forEach((control) => {
      control.disabled = false;
    });
  }

  function fillPlayerSelect(select, include, includeAbstain = false) {
    const previous = select.value;
    const options = [];
    if (includeAbstain) {
      options.push(option("", "棄權（不投給任何人）"));
    } else {
      options.push(option("", "請選擇玩家"));
    }
    for (const player of app.state.players.filter(include)) {
      options.push(option(player.id, player.name + (player.id === app.playerID ? "（你）" : "")));
    }
    select.replaceChildren(...options);
    if ([...select.options].some((item) => item.value === previous)) {
      select.value = previous;
    }
  }

  function option(value, label) {
    const item = document.createElement("option");
    item.value = value;
    item.textContent = label;
    return item;
  }

  function currentPlayer() {
    return playerByID(app.playerID);
  }

  function isCurrentOwner() {
    return app.private?.spectator !== true && app.state?.owner_id === app.playerID;
  }

  function playerByID(playerID) {
    return app.state?.players?.find((player) => player.id === playerID);
  }

  function available(eventType) {
    return app.private?.available?.includes(eventType) === true;
  }

  function copyInviteLink() {
    const url = new URL(window.location.href);
    url.search = new URLSearchParams({ room: app.roomID }).toString();
    navigator.clipboard.writeText(url.toString())
      .then(() => showToast("邀請連結已複製。"))
      .catch(() => showToast("無法存取剪貼簿，請直接複製網址列。", true));
  }

  function setConnection(state, label) {
    ui.connectionStatus.dataset.state = state;
    ui.connectionLabel.textContent = label;
  }

  function showJoinError(message) {
    ui.joinError.textContent = message;
    ui.joinError.hidden = false;
    ui.joinButton.disabled = false;
  }

  function hideJoinError() {
    ui.joinError.hidden = true;
    ui.joinError.textContent = "";
  }

  function showToast(message, isError = false) {
    window.clearTimeout(app.toastTimer);
    ui.toast.textContent = message;
    ui.toast.classList.toggle("error", isError);
    ui.toast.hidden = false;
    app.toastTimer = window.setTimeout(() => {
      ui.toast.hidden = true;
    }, 4500);
  }

  function translateError(message) {
    const translations = [
      ["reconnect token is required", "這個玩家席次需要重連憑證；已清除本機舊資料，請再加入一次。"],
      ["reconnect token is invalid", "重連憑證無效；已清除本機舊資料，請再加入一次。"],
      ["room is not joinable", "遊戲已經開始，目前無法加入新玩家。"],
      ["room is locked", "房間目前已鎖定，只允許既有席次重連。"],
      ["room player limit has been reached", "房間的玩家席次已滿。你仍可選擇以旁觀者加入未鎖定的房間。"],
      ["only the room owner", "只有房主可以執行這項操作。"],
      ["all non-owner players must be ready", "所有非房主玩家都必須先準備。"],
      ["not enough players", "玩家人數不足，至少需要兩名玩家。"],
      ["action is not allowed", "目前階段無法執行這項操作。"],
      ["player is dead", "你已出局，無法執行這項操作。"],
      ["spectators cannot chat", "旁觀者在對局進行中無法公開發言。"],
      ["participant type does not match", "保存的席次類型不符；已清除本機舊資料，請重新加入。"],
      ["participant not found", "這個席次已不存在。"],
      ["the room owner cannot be kicked", "房主不能移除自己，請先轉移房主。"],
      ["participants can only be kicked", "只能在等待室或遊戲結束後移除參與者。"],
      ["removed from room by the owner", "你已被房主移出房間。"],
      ["new owner must be a connected seated player", "新房主必須是目前在線的玩家。"],
      ["player limit is out of range", "玩家上限必須介於 2 到 20。"],
      ["player limit cannot be below", "玩家上限不能低於目前已就座人數。"],
      ["room is already waiting", "房間目前已在等待室。"],
      ["target is invalid", "所選目標已失效，請重新選擇。"],
      ["self target is not allowed", "這個能力不能以自己為目標。"],
      ["message is too long", "訊息超過 500 bytes，請縮短內容。"],
      ["chat event rate limit exceeded", "聊天訊息送得太快，請稍後再試。"],
      ["game event rate limit exceeded", "操作送得太快，請稍後再試。"],
      ["chat message rejected by moderation", "訊息未通過聊天室審核。"],
      ["chat moderation unavailable", "聊天室審核暫時無法使用，請稍後再試。"],
    ];
    return translations.find(([fragment]) => message.includes(fragment))?.[1] || message;
  }

  function updateResumeNote() {
    const roomID = ui.roomId.value.trim();
    const session = getSession(roomID);
    ui.joinAsSpectator.checked = session?.spectator === true;
    ui.joinAsSpectator.disabled = Boolean(session);
    ui.resumeNote.hidden = !session;
    ui.resumeNote.textContent = session
      ? `找到 ${session.playerName || "先前參與者"} 的${session.spectator ? "旁觀席" : "玩家席次"}重連資料，加入時將嘗試取回原席次。`
      : "";
    if (session?.playerName && !ui.playerName.value.trim()) {
      ui.playerName.value = session.playerName;
    }
  }

  function getSessions() {
    try {
      return JSON.parse(safeLocalStorageGet(SESSION_KEY) || "{}") || {};
    } catch {
      return {};
    }
  }

  function getSession(roomID) {
    return getSessions()[roomID] || null;
  }

  function saveSession(roomID, session) {
    const sessions = getSessions();
    sessions[roomID] = session;
    safeLocalStorageSet(SESSION_KEY, JSON.stringify(sessions));
  }

  function deleteSession(roomID) {
    const sessions = getSessions();
    delete sessions[roomID];
    safeLocalStorageSet(SESSION_KEY, JSON.stringify(sessions));
  }

  function safeLocalStorageGet(key) {
    try {
      return window.localStorage.getItem(key);
    } catch {
      return null;
    }
  }

  function safeLocalStorageSet(key, value) {
    try {
      window.localStorage.setItem(key, value);
    } catch {
      // The game still works for the current connection when storage is disabled.
    }
  }

  function randomRoomID() {
    const words = ["moon", "raven", "mist", "ember", "echo", "night", "council", "lantern"];
    const word = words[Math.floor(Math.random() * words.length)];
    const number = Math.floor(1000 + Math.random() * 9000);
    return `${word}-${number}`;
  }

  function newPlayerID() {
    if (window.crypto?.randomUUID) {
      return window.crypto.randomUUID();
    }
    return `player-${Date.now()}-${Math.random().toString(16).slice(2)}`;
  }

  function initials(name) {
    return [...(name || "?").trim()].slice(0, 2).join("").toUpperCase();
  }

  function formatTime(value) {
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) {
      return "";
    }
    return new Intl.DateTimeFormat("zh-TW", {
      hour: "2-digit",
      minute: "2-digit",
      hour12: false,
    }).format(date);
  }

  function element(tag, className = "", text = "") {
    const node = document.createElement(tag);
    if (className) {
      node.className = className;
    }
    node.textContent = text;
    return node;
  }

  start();
})();
