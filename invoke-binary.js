/**
 * based on:
 * https://full-stack.blend.com/how-we-write-github-actions-in-go/invoke-binary.js
 * which is referenced in:
 * https://full-stack.blend.com/how-we-write-github-actions-in-go.html
 */

const childProcess = require('child_process')
const os = require('os')
const process = require('process')

const VERSION = 'e5f66aff32'

function chooseBinary() {
    const platform = os.platform()
    const arch = os.arch()

    if (platform === 'linux' && arch === 'x64') {
        return `dist/main-linux-amd64-${VERSION}`
    }
    if (platform === 'linux' && arch === 'arm64') {
        return `dist/main-linux-arm64-${VERSION}`
    }
    // TODO: enable before v1
    // if (platform === 'windows' && arch === 'x64') {
    //     return `dist/main-windows-amd64-${VERSION}`
    // }
    // if (platform === 'windows' && arch === 'arm64') {
    //     return `dist/main-windows-arm64-${VERSION}`
    // }

    console.error(`Unsupported platform (${platform}) and architecture (${arch})`)
    process.exit(1)
}

function main() {
    const binary = chooseBinary()
    const mainScript = `${__dirname}/${binary}`
    const spawnSyncReturns = childProcess.spawnSync(mainScript, { stdio: 'inherit' })
    const status = spawnSyncReturns.status
    if (typeof status === 'number') {
        process.exit(status)
    }
    process.exit(1)
}

if (require.main === module) {
    main()
}
