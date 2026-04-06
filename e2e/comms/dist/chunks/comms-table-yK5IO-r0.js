//#region web/runtime/wasm/sqlite/async-opfs.ts
var COMMS_BROADCAST_CHANNEL = "bldr-comms-sqlite";
//#endregion
//#region web/bldr/comms-table.ts
var COMMS_DB_FILENAME = "comms.db";
var MESSAGES_TABLE = "messages";
var MESSAGES_DDL = `CREATE TABLE IF NOT EXISTS ${MESSAGES_TABLE} (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  source_plugin_id INTEGER NOT NULL,
  target_plugin_id INTEGER NOT NULL,
  payload BLOB NOT NULL,
  created_at INTEGER NOT NULL DEFAULT (unixepoch())
)`;
var CommsWriter = class {
	db;
	channel;
	seq = 0;
	constructor(db) {
		this.db = db;
		this.channel = new BroadcastChannel(COMMS_BROADCAST_CHANNEL);
		this.db.exec(MESSAGES_DDL);
	}
	write(sourcePluginId, targetPluginId, payload) {
		this.db.exec({
			sql: `INSERT INTO ${MESSAGES_TABLE} (source_plugin_id, target_plugin_id, payload) VALUES (?, ?, ?)`,
			bind: [
				sourcePluginId,
				targetPluginId,
				payload
			]
		});
		const id = Number(this.db.exec({
			sql: "SELECT last_insert_rowid()",
			returnValue: "resultRows"
		})[0][0]);
		this.seq++;
		const notification = {
			table: MESSAGES_TABLE,
			seq: this.seq
		};
		this.channel.postMessage(notification);
		return id;
	}
	close() {
		this.channel.close();
	}
};
var CommsReader = class {
	lastId = 0;
	readNew(db, targetPluginId) {
		const rows = db.exec({
			sql: `SELECT id, source_plugin_id, target_plugin_id, payload, created_at
            FROM ${MESSAGES_TABLE}
            WHERE target_plugin_id = ? AND id > ?
            ORDER BY id ASC`,
			bind: [targetPluginId, this.lastId],
			returnValue: "resultRows"
		});
		const messages = [];
		for (const row of rows) {
			const msg = {
				id: row[0],
				sourcePluginId: row[1],
				targetPluginId: row[2],
				payload: row[3],
				createdAt: row[4]
			};
			messages.push(msg);
			if (msg.id > this.lastId) this.lastId = msg.id;
		}
		return messages;
	}
	deleteConsumed(db, targetPluginId) {
		db.exec({
			sql: `DELETE FROM ${MESSAGES_TABLE} WHERE target_plugin_id = ? AND id <= ?`,
			bind: [targetPluginId, this.lastId]
		});
	}
};
function initCommsSchema(db) {
	db.exec(MESSAGES_DDL);
}
//#endregion
export { COMMS_BROADCAST_CHANNEL as a, initCommsSchema as i, CommsReader as n, CommsWriter as r, COMMS_DB_FILENAME as t };

//# sourceMappingURL=comms-table-yK5IO-r0.js.map