#!/usr/bin/env node

const net = require('net');
const { spawn, exec } = require('child_process');
const { promisify } = require('util');
const execAsync = promisify(exec);

// ANSI color codes
const colors = {
  green: '\x1b[32m',
  red: '\x1b[31m',
  reset: '\x1b[0m'
};

// Check if required arguments are provided
if (process.argv.length < 7) {
  console.log('\n╔════════════════════════════════════════════════════╗');
  console.log('║      PIZZAKV - COMPREHENSIVE TEST SUITE           ║');
  console.log('╚════════════════════════════════════════════════════╝');
  console.log('\nUsage: node test_comprehensive.js <binary> <port> <relaunch_wait> <command_wait> <cleanup_cmd>');
  console.log('\nParameters:');
  console.log('  binary         - Path to the pizzakv binary (e.g., ./pizzakv)');
  console.log('  port           - TCP port number (e.g., 8085)');
  console.log('  relaunch_wait  - Wait time before restarting server in ms (e.g., 2000)');
  console.log('  command_wait   - Wait time after server start in ms (e.g., 1000)');
  console.log('  cleanup_cmd    - Command to clean persistence files (e.g., "rm -f .db")');
  console.log('\nExample:');
  console.log('  node test_comprehensive.js ./pizzakv 8085 2000 1000 "rm -f .db"\n');
  process.exit(1);
}

// Configuration from command line arguments
const config = {
  binary: process.argv[2],
  port: parseInt(process.argv[3]),
  relaunchWait: parseInt(process.argv[4]),
  commandWait: parseInt(process.argv[5]),
  cleanupCmd: process.argv[6]
};

console.log('\n╔════════════════════════════════════════════════════╗');
console.log('║      PIZZAKV - COMPREHENSIVE TEST SUITE           ║');
console.log('╚════════════════════════════════════════════════════╝');
console.log(`\nConfiguration:`);
console.log(`  Binary:        ${config.binary}`);
console.log(`  Port:          ${config.port}`);
console.log(`  Relaunch Wait: ${config.relaunchWait}ms`);
console.log(`  Command Wait:  ${config.commandWait}ms`);
console.log(`  Cleanup Cmd:   ${config.cleanupCmd}\n`);

class KVDBClient {
  constructor(port = 8085) {
    this.port = port;
    this.socket = null;
    this.connected = false;
  }

  async connect(retries = 5, delay = 1000) {
    if (this.connected) return;
    
    for (let attempt = 1; attempt <= retries; attempt++) {
      try {
        await new Promise((resolve, reject) => {
          this.socket = new net.Socket();
          this.socket.setKeepAlive(true);
          this.socket.setNoDelay(true);
          
          const timeout = setTimeout(() => {
            this.socket.destroy();
            reject(new Error('Connection timeout'));
          }, 3000);
          
          this.socket.connect(this.port, 'localhost', () => {
            clearTimeout(timeout);
            this.connected = true;
            resolve();
          });
          
          this.socket.once('error', (err) => {
            clearTimeout(timeout);
            this.connected = false;
            reject(err);
          });
        });
        return;
      } catch (err) {
        if (attempt < retries) {
          console.log(`  Connection attempt ${attempt}/${retries} failed, retrying in ${delay}ms...`);
          await new Promise(resolve => setTimeout(resolve, delay));
        } else {
          throw new Error(`Failed to connect after ${retries} attempts`);
        }
      }
    }
  }

  disconnect() {
    if (this.socket) {
      this.socket.destroy();
      this.socket = null;
      this.connected = false;
    }
  }

  sendCommand(command) {
    return new Promise(async (resolve, reject) => {
      if (!this.socket || !this.connected) {
        try {
          await this.connect();
        } catch (err) {
          return reject(err);
        }
      }
      
      let response = '';

      const dataHandler = (data) => {
        response += data.toString();
        
        if (response.includes('\r')) {
          this.socket.removeListener('data', dataHandler);
          clearTimeout(timeout);
          const result = response.endsWith('\r') ? response.slice(0, -1) : response;
          resolve(result);
        }
      };

      this.socket.on('data', dataHandler);

      const timeout = setTimeout(() => {
        this.socket.removeListener('data', dataHandler);
        reject(new Error('Command timeout'));
      }, 30000); // Increased from 5000 to 30000ms for KEYS/READS with large datasets
      
      this.socket.write(command, (err) => {
        if (err) {
          this.socket.removeListener('data', dataHandler);
          clearTimeout(timeout);
          reject(err);
        }
      });
    });
  }

  async write(key, value) {
    return this.sendCommand(`write ${key}|${value}\r`);
  }

  async read(key) {
    return this.sendCommand(`read ${key}\r`);
  }

  async delete(key) {
    return this.sendCommand(`delete ${key}\r`);
  }

  async keys() {
    return this.sendCommand(`keys\r`);
  }

  async reads(prefix) {
    return this.sendCommand(`reads ${prefix}\r`);
  }

  async status() {
    return this.sendCommand(`status\r`);
  }
}

class ServerManager {
  constructor(binary, port) {
    this.binary = binary;
    this.port = port;
    this.process = null;
  }

  async cleanPersistence() {
    if (!config.cleanupCmd) {
      console.log('⊘ No cleanup command specified, skipping...');
      return;
    }

    try {
      console.log(`\n🧹 Cleaning persistence: ${config.cleanupCmd}`);
      await execAsync(config.cleanupCmd);
      console.log('✓ Persistence cleaned');
    } catch (err) {
      // Ignore errors if file doesn't exist
      if (err.code === 'ENOENT' || err.message.includes('No such file')) {
        console.log('✓ No persistence files to clean');
      } else {
        console.log(`⚠ Cleanup warning: ${err.message}`);
      }
    }
  }

  async start() {
    return new Promise((resolve, reject) => {
      console.log(`\n🚀 Starting server: ${this.binary}`);
      console.log('━━━ SERVER OUTPUT ━━━');
      
      this.process = spawn(this.binary, [], {
        stdio: ['ignore', 'pipe', 'pipe'],
        detached: false
      });

      let silenceTimer = null;
      const startupTimeout = setTimeout(() => {
        reject(new Error('Server startup timeout'));
      }, 10000);

      const resetSilenceTimer = () => {
        if (silenceTimer) clearTimeout(silenceTimer);
        silenceTimer = setTimeout(() => {
          // 1 second of silence - server is ready
          clearTimeout(startupTimeout);
          this.process.stdout.removeListener('data', onData);
          this.process.stderr.removeListener('data', onErrorData);
          console.log('━━━━━━━━━━━━━━━━━━━━━');
          console.log(`✓ Server started (PID: ${this.process.pid})`);
          
          // Continue logging in background
          this.setupContinuousLogging();
          resolve();
        }, 1000);
      };

      const onData = (data) => {
        const output = data.toString();
        const lines = output.split('\n');
        
        lines.forEach((line, index) => {
          if (!line && index === lines.length - 1) return; // Skip final empty line
          
          // If line contains \r, handle each segment as an overwrite
          if (line.includes('\r')) {
            const segments = line.split('\r');
            segments.forEach((segment, segIndex) => {
              if (segment.trim()) {
                // Use \r to overwrite the current line for all segments except the last
                if (segIndex === segments.length - 1) {
                  // Last segment gets a newline
                  console.log(`${colors.green}  [SERVER] ${segment}${colors.reset}`);
                } else {
                  // Intermediate segments overwrite
                  process.stdout.write(`\r${colors.green}  [SERVER] ${segment}${colors.reset}`);
                }
              }
            });
          } else if (line.trim()) {
            // Normal line - print with newline
            console.log(`${colors.green}  [SERVER] ${line}${colors.reset}`);
          }
        });
        
        resetSilenceTimer();
      };

      const onErrorData = (data) => {
        const output = data.toString();
        // Log all stderr as regular server output in green
        output.split('\n').forEach(line => {
          if (line.trim()) {
            console.log(`${colors.green}  [SERVER] ${line}${colors.reset}`);
          }
        });
        resetSilenceTimer();
      };

      this.process.stdout.on('data', onData);
      this.process.stderr.on('data', onErrorData);

      this.process.on('error', (err) => {
        if (silenceTimer) clearTimeout(silenceTimer);
        clearTimeout(startupTimeout);
        console.log(`${colors.red}\n⚠ SERVER ERROR: ${err.message}${colors.reset}`);
        reject(new Error(`Failed to start server: ${err.message}`));
      });

      this.process.on('exit', (code, signal) => {
        if (code !== null && code !== 0 && !this.intentionalKill) {
          console.log(`${colors.red}\n⚠ SERVER CRASHED: Exited unexpectedly with code ${code}${signal ? ` (signal: ${signal})` : ''}${colors.reset}`);
          if (silenceTimer) clearTimeout(silenceTimer);
          clearTimeout(startupTimeout);
        }
      });

      // Start the silence timer
      resetSilenceTimer();
    });
  }

  setupContinuousLogging() {
    if (!this.process) return;
    
    this.process.stdout.on('data', (data) => {
      const output = data.toString();
      const lines = output.split('\n');
      
      lines.forEach((line, index) => {
        if (!line && index === lines.length - 1) return; // Skip final empty line
        
        // If line contains \r, handle each segment as an overwrite
        if (line.includes('\r')) {
          const segments = line.split('\r');
          segments.forEach((segment, segIndex) => {
            if (segment.trim()) {
              // Use \r to overwrite the current line for all segments except the last
              if (segIndex === segments.length - 1) {
                // Last segment gets a newline
                console.log(`${colors.green}  [SERVER] ${segment}${colors.reset}`);
              } else {
                // Intermediate segments overwrite
                process.stdout.write(`\r${colors.green}  [SERVER] ${segment}${colors.reset}`);
              }
            }
          });
        } else if (line.trim()) {
          // Normal line - print with newline
          console.log(`${colors.green}  [SERVER] ${line}${colors.reset}`);
        }
      });
    });

    this.process.stderr.on('data', (data) => {
      const output = data.toString();
      const lines = output.split('\n');
      
      lines.forEach((line, index) => {
        if (!line && index === lines.length - 1) return; // Skip final empty line
        
        // If line contains \r, handle each segment as an overwrite
        if (line.includes('\r')) {
          const segments = line.split('\r');
          segments.forEach((segment, segIndex) => {
            if (segment.trim()) {
              // Use \r to overwrite the current line for all segments except the last
              if (segIndex === segments.length - 1) {
                // Last segment gets a newline
                console.log(`${colors.green}  [SERVER] ${segment}${colors.reset}`);
              } else {
                // Intermediate segments overwrite
                process.stdout.write(`\r${colors.green}  [SERVER] ${segment}${colors.reset}`);
              }
            }
          });
        } else if (line.trim()) {
          console.log(`${colors.green}  [SERVER] ${line}${colors.reset}`);
        }
      });
    });
  }

  async stop() {
    if (!this.process) {
      console.log('⚠ No server process to stop');
      return;
    }

    return new Promise((resolve) => {
      console.log(`\n🛑 Stopping server (PID: ${this.process.pid})`);
      this.intentionalKill = true;

      const killTimeout = setTimeout(() => {
        if (this.process && !this.process.killed) {
          console.log('⚠ Force killing server (SIGKILL)...');
          this.process.kill('SIGKILL');
        }
      }, 10000);

      this.process.once('exit', () => {
        clearTimeout(killTimeout);
        console.log('✓ Server stopped');
        this.process = null;
        this.intentionalKill = false;
        resolve();
      });

      this.process.kill('SIGTERM');
    });
  }
}

// Test data that will be used for persistence testing
const testData = {
  basic: [
    { key: 'test:write1', value: 'value1' },
    { key: 'test:write2', value: 'value2' },
    { key: 'test:write3', value: 'value3' }
  ],
  prefixes: [
    { key: 'user:alice', value: 'Alice Johnson' },
    { key: 'user:bob', value: 'Bob Smith' },
    { key: 'user:charlie', value: 'Charlie Brown' },
    { key: 'product:apple', value: 'Fresh Apple' },
    { key: 'product:banana', value: 'Yellow Banana' },
    { key: 'order:001', value: 'Order #001' },
    { key: 'order:002', value: 'Order #002' }
  ],
  toDelete: [
    { key: 'temp:delete1', value: 'temporary1' },
    { key: 'temp:delete2', value: 'temporary2' }
  ]
};

async function testWrite(client) {
  console.log('\n━━━ TEST: WRITE ━━━');
  let passed = true;

  for (const item of testData.basic) {
    try {
      const result = await client.write(item.key, item.value);
      if (result === 'ok' || result === 'success' || result.toLowerCase().includes('ok')) {
        console.log(`  ✓ Write ${item.key}: ${result}`);
      } else {
        console.log(`  ⚠ Write ${item.key}: unexpected response "${result}"`);
      }
    } catch (err) {
      console.log(`  ✗ Write ${item.key} failed: ${err.message}`);
      passed = false;
    }
  }

  // Write additional test data
  for (const item of [...testData.prefixes, ...testData.toDelete]) {
    try {
      await client.write(item.key, item.value);
    } catch (err) {
      console.log(`  ✗ Write ${item.key} failed: ${err.message}`);
      passed = false;
    }
  }

  return passed;
}

async function testRead(client) {
  console.log('\n━━━ TEST: READ ━━━');
  let passed = true;

  for (const item of testData.basic) {
    try {
      const result = await client.read(item.key);
      if (result === item.value) {
        console.log(`  ✓ Read ${item.key}: ${result}`);
      } else {
        console.log(`  ✗ Read ${item.key}: expected "${item.value}", got "${result}"`);
        passed = false;
      }
    } catch (err) {
      console.log(`  ✗ Read ${item.key} failed: ${err.message}`);
      passed = false;
    }
  }

  // Test reading non-existent key
  try {
    const result = await client.read('nonexistent:key');
    if (result === 'error' || result === '') {
      console.log(`  ✓ Read nonexistent key: correctly returned "${result}"`);
    } else {
      console.log(`  ⚠ Read nonexistent key: unexpected response "${result}"`);
    }
  } catch (err) {
    console.log(`  ✓ Read nonexistent key: correctly threw error`);
  }

  return passed;
}

async function testDelete(client) {
  console.log('\n━━━ TEST: DELETE ━━━');
  let passed = true;

  for (const item of testData.toDelete) {
    try {
      const result = await client.delete(item.key);
      if (result === 'ok' || result === 'success' || result.toLowerCase().includes('ok')) {
        console.log(`  ✓ Delete ${item.key}: ${result}`);
        
        // Verify deletion
        const readResult = await client.read(item.key);
        if (readResult === 'error' || readResult === '') {
          console.log(`  ✓ Verified ${item.key} was deleted`);
        } else {
          console.log(`  ✗ ${item.key} still exists after deletion: ${readResult}`);
          passed = false;
        }
      } else {
        console.log(`  ⚠ Delete ${item.key}: unexpected response "${result}"`);
      }
    } catch (err) {
      console.log(`  ✗ Delete ${item.key} failed: ${err.message}`);
      passed = false;
    }
  }

  // Test deleting non-existent key
  try {
    const result = await client.delete('nonexistent:key');
    if (result === 'error') {
      console.log(`  ✓ Delete nonexistent key: correctly returned error`);
    } else {
      console.log(`  ⚠ Delete nonexistent key: unexpected response "${result}"`);
    }
  } catch (err) {
    console.log(`  ✓ Delete nonexistent key: correctly handled`);
  }

  return passed;
}

async function testKeys(client) {
  console.log('\n━━━ TEST: KEYS ━━━');
  let passed = true;

  try {
    const result = await client.keys();
    
    if (result === 'error') {
      console.log(`  ⊘ KEYS command not supported`);
      return null; // null means not supported
    }

    const keysList = result.split('\r').filter(k => k.trim());
    console.log(`  ✓ Found ${keysList.length} keys`);
    
    // Verify some expected keys exist
    const expectedKeys = ['test:write1', 'user:alice', 'product:apple'];
    for (const expectedKey of expectedKeys) {
      if (keysList.includes(expectedKey)) {
        console.log(`  ✓ Found expected key: ${expectedKey}`);
      } else {
        console.log(`  ⚠ Expected key not found: ${expectedKey}`);
      }
    }

    // Verify deleted keys don't exist
    for (const item of testData.toDelete) {
      if (!keysList.includes(item.key)) {
        console.log(`  ✓ Deleted key not in list: ${item.key}`);
      } else {
        console.log(`  ✗ Deleted key still in list: ${item.key}`);
        passed = false;
      }
    }

  } catch (err) {
    console.log(`  ⊘ KEYS command not supported: ${err.message}`);
    return null;
  }

  return passed;
}

async function testReads(client) {
  console.log('\n━━━ TEST: READS ━━━');
  let passed = true;

  const prefixes = ['user', 'product', 'order'];
  
  for (const prefix of prefixes) {
    try {
      const result = await client.reads(prefix);
      
      if (result === 'error') {
        console.log(`  ⊘ READS command not supported`);
        return null;
      }

      const values = result.split('\r').filter(v => v.trim());
      console.log(`  ✓ Reads with prefix "${prefix}": found ${values.length} values`);
      
      // Show first few values
      values.slice(0, 3).forEach(v => console.log(`    - ${v}`));
      if (values.length > 3) {
        console.log(`    ... and ${values.length - 3} more`);
      }

    } catch (err) {
      console.log(`  ⊘ READS command not supported: ${err.message}`);
      return null;
    }
  }

  // Test non-existent prefix
  try {
    const result = await client.reads('nonexistent');
    if (result === 'error' || result === '') {
      console.log(`  ✓ Nonexistent prefix correctly returned empty`);
    }
  } catch (err) {
    console.log(`  ✓ Nonexistent prefix correctly handled`);
  }

  return passed;
}

async function testStatus(client) {
  console.log('\n━━━ TEST: STATUS ━━━');
  let passed = true;

  try {
    const result = await client.status();
    console.log(`  ✓ Status: ${result}`);
  } catch (err) {
    console.log(`  ✗ Status failed: ${err.message}`);
    passed = false;
  }

  return passed;
}

async function testPersistence(client) {
  console.log('\n━━━ TEST: PERSISTENCE VERIFICATION ━━━');
  let passed = true;

  // Verify data that should have persisted
  console.log('\nVerifying persisted data:');
  const persistedData = testData.basic.concat(testData.prefixes);
  
  for (const item of persistedData) {
    try {
      const result = await client.read(item.key);
      if (result === item.value) {
        console.log(`  ✓ Persisted ${item.key}: ${result}`);
      } else {
        console.log(`  ✗ Persisted ${item.key}: expected "${item.value}", got "${result}"`);
        passed = false;
      }
    } catch (err) {
      console.log(`  ✗ Failed to read persisted key ${item.key}: ${err.message}`);
      passed = false;
    }
  }

  // Verify deleted data is still deleted
  console.log('\nVerifying deleted data is still gone:');
  for (const item of testData.toDelete) {
    try {
      const result = await client.read(item.key);
      if (result === 'error' || result === '') {
        console.log(`  ✓ Deleted key ${item.key} is still deleted`);
      } else {
        console.log(`  ✗ Deleted key ${item.key} reappeared with value: ${result}`);
        passed = false;
      }
    } catch (err) {
      console.log(`  ✓ Deleted key ${item.key} is still deleted`);
    }
  }

  return passed;
}

async function stressTest(client, level) {
  const { writes, reads, deletes, label } = level;
  const startTime = Date.now();
  const results = {
    writes: { success: 0, errors: 0, time: 0, ops: 0 },
    reads: { success: 0, errors: 0, validationErrors: 0, time: 0, ops: 0 },
    deletes: { success: 0, errors: 0, time: 0, ops: 0 },
    keys: { time: 0, count: 0 },
    readsCmd: { time: 0, count: 0 }
  };

  console.log(`\n${'━'.repeat(60)}`);
  console.log(`  LEVEL: ${label}`);
  console.log(`  ${writes.toLocaleString()} writes | ${reads.toLocaleString()} reads | ${deletes.toLocaleString()} deletes`);
  console.log(`${'━'.repeat(60)}`);

  // WRITE TEST
  console.log(`\n📝 Writing ${writes.toLocaleString()} records...`);
  const writeStart = Date.now();
  for (let i = 0; i < writes; i++) {
    try {
      const key = `stress:${label}:${i}`;
      const value = `value_${label}_${i}`;
      await client.write(key, value);
      results.writes.success++;

      if ((i + 1) % Math.max(1, Math.floor(writes / 10)) === 0) {
        const elapsed = (Date.now() - writeStart) / 1000;
        const rate = Math.floor((i + 1) / elapsed);
        const progress = ((i + 1) / writes * 100).toFixed(0);
        process.stdout.write(`\r  Progress: ${(i + 1).toLocaleString()}/${writes.toLocaleString()} | ${rate.toLocaleString()} ops/s | ${progress}%     `);
      }
    } catch (err) {
      results.writes.errors++;
      if (!client.connected) await client.connect();
    }
  }
  results.writes.time = (Date.now() - writeStart) / 1000;
  results.writes.ops = Math.floor(results.writes.success / results.writes.time);
  console.log(`\n  ✓ Writes: ${results.writes.success.toLocaleString()} success, ${results.writes.errors} errors | ${results.writes.time.toFixed(2)}s | ${results.writes.ops.toLocaleString()} ops/s`);

  // READ TEST
  console.log(`\n📖 Reading and validating ${reads.toLocaleString()} records...`);
  const readStart = Date.now();
  for (let i = 0; i < reads; i++) {
    try {
      const key = `stress:${label}:${i}`;
      const expectedValue = `value_${label}_${i}`;
      const actualValue = await client.read(key);
      
      if (actualValue !== expectedValue) {
        results.reads.validationErrors++;
        if (results.reads.validationErrors <= 3) {
          console.log(`\n  ✗ Validation failed for ${key}: expected "${expectedValue}", got "${actualValue}"`);
        }
      }
      results.reads.success++;

      if ((i + 1) % Math.max(1, Math.floor(reads / 10)) === 0) {
        const elapsed = (Date.now() - readStart) / 1000;
        const rate = Math.floor((i + 1) / elapsed);
        const progress = ((i + 1) / reads * 100).toFixed(0);
        process.stdout.write(`\r  Progress: ${(i + 1).toLocaleString()}/${reads.toLocaleString()} | ${rate.toLocaleString()} reads/s | ${progress}% | validation errors: ${results.reads.validationErrors}     `);
      }
    } catch (err) {
      results.reads.errors++;
      if (!client.connected) await client.connect();
    }
  }
  results.reads.time = (Date.now() - readStart) / 1000;
  results.reads.ops = Math.floor(results.reads.success / results.reads.time);
  console.log(`\n  ✓ Reads: ${results.reads.success.toLocaleString()} success, ${results.reads.errors} errors, ${results.reads.validationErrors} validation errors | ${results.reads.time.toFixed(2)}s | ${results.reads.ops.toLocaleString()} reads/s`);

  // DELETE TEST
  console.log(`\n🗑️  Deleting ${deletes.toLocaleString()} records...`);
  const deleteStart = Date.now();
  for (let i = 0; i < deletes; i++) {
    try {
      const key = `stress:${label}:${i}`;
      await client.delete(key);
      results.deletes.success++;

      if ((i + 1) % Math.max(1, Math.floor(deletes / 10)) === 0) {
        const elapsed = (Date.now() - deleteStart) / 1000;
        const rate = Math.floor((i + 1) / elapsed);
        const progress = ((i + 1) / deletes * 100).toFixed(0);
        process.stdout.write(`\r  Progress: ${(i + 1).toLocaleString()}/${deletes.toLocaleString()} | ${rate.toLocaleString()} ops/s | ${progress}%     `);
      }
    } catch (err) {
      results.deletes.errors++;
      if (!client.connected) await client.connect();
    }
  }
  results.deletes.time = (Date.now() - deleteStart) / 1000;
  results.deletes.ops = Math.floor(results.deletes.success / results.deletes.time);
  console.log(`\n  ✓ Deletes: ${results.deletes.success.toLocaleString()} success, ${results.deletes.errors} errors | ${results.deletes.time.toFixed(2)}s | ${results.deletes.ops.toLocaleString()} ops/s`);

  // KEYS TEST
  console.log(`\n🔑 Testing KEYS command...`);
  const keysStart = Date.now();
  try {
    const keysResult = await client.keys();
    if (keysResult && keysResult !== 'error' && keysResult.trim() !== '') {
      const keysList = keysResult.split('\r').filter(k => k.trim());
      results.keys.count = keysList.length;
      results.keys.time = (Date.now() - keysStart) / 1000;
      console.log(`  ✓ KEYS: ${results.keys.count.toLocaleString()} keys returned | ${results.keys.time.toFixed(3)}s`);
    } else {
      console.log(`  ⊘ KEYS command not supported or returned empty`);
    }
  } catch (err) {
    console.log(`  ⊘ KEYS command error: ${err.message}`);
  }

  // READS (prefix) TEST
  console.log(`\n📚 Testing READS command...`);
  const readsStart = Date.now();
  try {
    const readsResult = await client.reads(`stress:${label}`);
    if (readsResult && readsResult !== 'error' && readsResult.trim() !== '') {
      const valuesList = readsResult.split('\r').filter(v => v.trim());
      results.readsCmd.count = valuesList.length;
      results.readsCmd.time = (Date.now() - readsStart) / 1000;
      console.log(`  ✓ READS: ${results.readsCmd.count.toLocaleString()} values returned | ${results.readsCmd.time.toFixed(3)}s`);
    } else {
      console.log(`  ⊘ READS command not supported or returned empty`);
    }
  } catch (err) {
    console.log(`  ⊘ READS command error: ${err.message}`);
  }

  const totalTime = (Date.now() - startTime) / 1000;
  console.log(`\n  📊 LEVEL SUMMARY: ${totalTime.toFixed(2)}s total`);
  console.log(`     Writes:  ${results.writes.ops.toLocaleString().padStart(8)} ops/s`);
  console.log(`     Reads:   ${results.reads.ops.toLocaleString().padStart(8)} reads/s`);
  console.log(`     Deletes: ${results.deletes.ops.toLocaleString().padStart(8)} ops/s`);

  return results;
}

async function runStressTests() {
  const server = new ServerManager(config.binary, config.port);
  const client = new KVDBClient(config.port);

  // Define progressive stress levels
  const levels = [
    { label: 'L1_1K', writes: 1000, reads: 1000, deletes: 100 },
    { label: 'L2_10K', writes: 10000, reads: 10000, deletes: 1000 },
    { label: 'L3_50K', writes: 50000, reads: 50000, deletes: 5000 }
//    { label: 'L4_100K', writes: 100000, reads: 100000, deletes: 10000 },
//    { label: 'L5_500K', writes: 500000, reads: 500000, deletes: 50000 },
//    { label: 'L6_1M', writes: 1000000, reads: 1000000, deletes: 100000 }
  ];

  const allResults = [];

  try {
    console.log('\n' + '╔' + '═'.repeat(58) + '╗');
    console.log('║' + ' '.repeat(15) + 'STRESS TEST MODE' + ' '.repeat(27) + '║');
    console.log('╚' + '═'.repeat(58) + '╝');

    // Clean persistence before stress test
    console.log('\n' + '═'.repeat(60));
    console.log('CLEANING PERSISTENCE FOR STRESS TEST');
    console.log('═'.repeat(60));
    await server.cleanPersistence();

    // Start server
    console.log('\n' + '═'.repeat(60));
    console.log('INITIALIZING SERVER');
    console.log('═'.repeat(60));

    await server.start();
    await new Promise(resolve => setTimeout(resolve, config.commandWait));
    await client.connect();
    console.log('✓ Server started and client connected');

    // Run each stress level
    for (let i = 0; i < levels.length; i++) {
      const level = levels[i];
      console.log('\n' + '═'.repeat(60));
      console.log(`STRESS LEVEL ${i + 1}/${levels.length}: ${level.label.toUpperCase()}`);
      console.log('═'.repeat(60));

      const results = await stressTest(client, level);
      allResults.push({ level: level.label, results });

      // After each level (except the last), test persistence
      if (i < levels.length - 1) {
        console.log('\n' + '═'.repeat(60));
        console.log('TESTING PERSISTENCE (Restart & Verify)');
        console.log('═'.repeat(60));

        const remainingKeys = level.writes - level.deletes;
        console.log(`\nExpecting ${remainingKeys.toLocaleString()} keys to persist (${level.writes.toLocaleString()} written - ${level.deletes.toLocaleString()} deleted)`);

        client.disconnect();
        await server.stop();
        await new Promise(resolve => setTimeout(resolve, 1000));

        console.log(`\n⏳ Waiting ${config.relaunchWait}ms before restarting...`);
        await server.start();
        await new Promise(resolve => setTimeout(resolve, config.relaunchWait));
        await client.connect();
        console.log('✓ Server restarted and client reconnected');

        // Verify a sample of persisted data
        console.log(`\n🔍 Verifying persistence (sampling 100 keys)...`);
        const sampleSize = Math.min(100, remainingKeys);
        let persistedCorrect = 0;
        let persistedErrors = 0;

        for (let j = 0; j < sampleSize; j++) {
          // Sample keys that should NOT have been deleted
          const keyIndex = level.deletes + Math.floor((remainingKeys - 1) * (j / sampleSize));
          const key = `stress:${level.label}:${keyIndex}`;
          const expectedValue = `value_${level.label}_${keyIndex}`;

          try {
            const actualValue = await client.read(key);
            if (actualValue === expectedValue) {
              persistedCorrect++;
            } else {
              persistedErrors++;
              if (persistedErrors <= 3) {
                console.log(`  ✗ Key ${key}: expected "${expectedValue}", got "${actualValue}"`);
              }
            }
          } catch (err) {
            persistedErrors++;
            if (persistedErrors <= 3) {
              console.log(`  ✗ Key ${key}: read failed - ${err.message}`);
            }
          }
        }

        console.log(`  ✓ Persistence check: ${persistedCorrect}/${sampleSize} correct (${persistedErrors} errors)`);

        // Verify deleted keys are still gone
        console.log(`\n🗑️  Verifying deletions (sampling 50 deleted keys)...`);
        const deleteSampleSize = Math.min(50, level.deletes);
        let deletedCorrect = 0;

        for (let j = 0; j < deleteSampleSize; j++) {
          const keyIndex = Math.floor(level.deletes * (j / deleteSampleSize));
          const key = `stress:${level.label}:${keyIndex}`;

          try {
            const result = await client.read(key);
            if (result === 'error' || result === '') {
              deletedCorrect++;
            } else {
              console.log(`  ✗ Deleted key ${key} still exists with value: ${result}`);
            }
          } catch (err) {
            deletedCorrect++;
          }
        }

        console.log(`  ✓ Deletion check: ${deletedCorrect}/${deleteSampleSize} confirmed deleted`);
      }
    }

    // Final cleanup
    console.log('\n' + '═'.repeat(60));
    console.log('CLEANUP');
    console.log('═'.repeat(60));

    client.disconnect();
    await server.stop();

    // Print final summary
    console.log('\n' + '╔' + '═'.repeat(78) + '╗');
    console.log('║' + ' '.repeat(28) + 'STRESS TEST SUMMARY' + ' '.repeat(31) + '║');
    console.log('╠' + '═'.repeat(78) + '╣');
    console.log('║ Level     │ Writes (ops/s) │ Reads (reads/s) │ Deletes (ops/s) │ Val.Errors ║');
    console.log('╠' + '═'.repeat(78) + '╣');

    allResults.forEach(({ level, results }) => {
      const levelPad = level.padEnd(9);
      const writesOps = results.writes.ops.toLocaleString().padStart(14);
      const readsOps = results.reads.ops.toLocaleString().padStart(15);
      const deletesOps = results.deletes.ops.toLocaleString().padStart(15);
      const valErrors = results.reads.validationErrors.toString().padStart(10);
      console.log(`║ ${levelPad} │ ${writesOps} │ ${readsOps} │ ${deletesOps} │ ${valErrors} ║`);
    });

    console.log('╚' + '═'.repeat(78) + '╝\n');

    // Check if all validation errors are 0
    const totalValidationErrors = allResults.reduce((sum, r) => sum + r.results.reads.validationErrors, 0);
    
    if (totalValidationErrors === 0) {
      console.log('🎉 ALL STRESS TESTS PASSED WITH 100% ACCURACY!\n');
      return true;
    } else {
      console.log(`⚠️  COMPLETED WITH ${totalValidationErrors} VALIDATION ERRORS\n`);
      return false;
    }

  } catch (err) {
    console.error(`\n❌ FATAL ERROR: ${err.message}\n`);
    console.error(err.stack);
    
    client.disconnect();
    if (server.process) {
      await server.stop();
    }
    
    throw err;
  }
}

async function runTests() {
  const server = new ServerManager(config.binary, config.port);
  const client = new KVDBClient(config.port);
  const results = {
    write: false,
    read: false,
    delete: false,
    keys: null,
    reads: null,
    status: false,
    persistence: false
  };

  try {
    // Clean persistence before starting
    console.log('\n' + '═'.repeat(60));
    console.log('INITIAL CLEANUP');
    console.log('═'.repeat(60));
    await server.cleanPersistence();

    // Phase 1: Start server and run initial tests
    console.log('\n' + '═'.repeat(60));
    console.log('PHASE 1: INITIAL SERVER STARTUP & OPERATIONS');
    console.log('═'.repeat(60));

    await server.start();
    await new Promise(resolve => setTimeout(resolve, config.commandWait));

    await client.connect();
    console.log('✓ Client connected\n');

    results.status = await testStatus(client);
    results.write = await testWrite(client);
    results.read = await testRead(client);
    results.delete = await testDelete(client);
    results.keys = await testKeys(client);
    results.reads = await testReads(client);

    // Phase 2: Stop server
    console.log('\n' + '═'.repeat(60));
    console.log('PHASE 2: SERVER SHUTDOWN');
    console.log('═'.repeat(60));

    client.disconnect();
    await server.stop();
    await new Promise(resolve => setTimeout(resolve, 1000));

    // Phase 3: Restart server and test persistence
    console.log('\n' + '═'.repeat(60));
    console.log('PHASE 3: SERVER RESTART & PERSISTENCE VERIFICATION');
    console.log('═'.repeat(60));

    await server.start();
    console.log(`⏳ Waiting ${config.relaunchWait}ms before reconnecting...`);
    await new Promise(resolve => setTimeout(resolve, config.relaunchWait));

    await client.connect();
    console.log('✓ Client reconnected\n');

    results.persistence = await testPersistence(client);

    // Final cleanup
    console.log('\n' + '═'.repeat(60));
    console.log('CLEANUP');
    console.log('═'.repeat(60));

    client.disconnect();
    await server.stop();

    // Print summary
    console.log('\n' + '╔' + '═'.repeat(58) + '╗');
    console.log('║' + ' '.repeat(20) + 'TEST SUMMARY' + ' '.repeat(26) + '║');
    console.log('╠' + '═'.repeat(58) + '╣');
    
    const printResult = (name, result) => {
      let status;
      if (result === true) status = '✓ PASS';
      else if (result === false) status = '✗ FAIL';
      else status = '⊘ N/A ';
      console.log(`║  ${status}  ${name.padEnd(48)} ║`);
    };

    printResult('WRITE', results.write);
    printResult('READ', results.read);
    printResult('DELETE', results.delete);
    printResult('KEYS', results.keys);
    printResult('READS', results.reads);
    printResult('STATUS', results.status);
    printResult('PERSISTENCE', results.persistence);

    console.log('╚' + '═'.repeat(58) + '╝\n');

    // Determine overall success
    const criticalTests = [results.write, results.read, results.persistence];
    const allCriticalPassed = criticalTests.every(r => r === true);

    if (allCriticalPassed) {
      console.log('🎉 ALL CRITICAL TESTS PASSED!\n');
      return true;
    } else {
      console.log('⚠️  SOME TESTS FAILED\n');
      return false;
    }

  } catch (err) {
    console.error(`\n❌ FATAL ERROR: ${err.message}\n`);
    console.error(err.stack);
    
    client.disconnect();
    if (server.process) {
      await server.stop();
    }
    
    throw err;
  }
}

// Handle process termination
process.on('SIGINT', () => {
  console.log('\n\n🛑 Received SIGINT, cleaning up...');
  process.exit(130);
});

process.on('SIGTERM', () => {
  console.log('\n\n🛑 Received SIGTERM, cleaning up...');
  process.exit(143);
});

// Main function to run both test suites
async function runAllTests() {
  let correctnessPassed = false;
  let stressPassed = false;
  
  try {
    // First run correctness tests
    console.log('\n' + '╔' + '═'.repeat(58) + '╗');
    console.log('║' + ' '.repeat(10) + 'STARTING CORRECTNESS TEST SUITE' + ' '.repeat(16) + '║');
    console.log('╚' + '═'.repeat(58) + '╝\n');
    
    correctnessPassed = await runTests();
    
    // Wait a bit between test suites
    await new Promise(resolve => setTimeout(resolve, 2000));
    
    // Then run stress tests
    console.log('\n\n' + '╔' + '═'.repeat(58) + '╗');
    console.log('║' + ' '.repeat(13) + 'STARTING STRESS TEST SUITE' + ' '.repeat(19) + '║');
    console.log('╚' + '═'.repeat(58) + '╝\n');
    
    stressPassed = await runStressTests();
    
    // Final summary
    console.log('\n' + '╔' + '═'.repeat(58) + '╗');
    console.log('║' + ' '.repeat(18) + 'FINAL SUMMARY' + ' '.repeat(27) + '║');
    console.log('╠' + '═'.repeat(58) + '╣');
    console.log(`║  ${correctnessPassed ? '✓ PASS' : '✗ FAIL'}  Correctness Tests${' '.repeat(33)} ║`);
    console.log(`║  ${stressPassed ? '✓ PASS' : '✗ FAIL'}  Stress Tests${' '.repeat(38)} ║`);
    console.log('╚' + '═'.repeat(58) + '╝\n');
    
    if (correctnessPassed && stressPassed) {
      console.log('🎉 ALL TESTS PASSED!\n');
      process.exit(0);
    } else {
      console.log('⚠️  SOME TESTS FAILED\n');
      process.exit(1);
    }
    
  } catch (err) {
    console.error(`\n❌ Test suite error: ${err.message}\n`);
    process.exit(1);
  }
}

// Run all tests
runAllTests().catch(err => {
  console.error(`\n❌ Unhandled error: ${err.message}\n`);
  process.exit(1);
});
