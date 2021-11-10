package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sync"

	config "github.com/ipfs/go-ipfs-config"
	files "github.com/ipfs/go-ipfs-files"
	icore "github.com/ipfs/interface-go-ipfs-core"
	iface "github.com/ipfs/interface-go-ipfs-core"
	icorepath "github.com/ipfs/interface-go-ipfs-core/path"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/ipfs/go-ipfs/core"
	"github.com/ipfs/go-ipfs/core/coreapi"
	"github.com/ipfs/go-ipfs/core/node"
	"github.com/ipfs/go-ipfs/core/node/libp2p" // This package is needed so that all the preloaded plugins are loaded automatically
	"github.com/ipfs/go-ipfs/plugin/loader"
	"github.com/ipfs/go-ipfs/repo/fsrepo"
	"github.com/libp2p/go-libp2p-core/peer"
)

func setupPlugins(externalPluginsPath string) error {
	// Load any external plugins if available on externalPluginsPath
	plugins, err := loader.NewPluginLoader(filepath.Join(externalPluginsPath, "plugins"))
	if err != nil {
		return fmt.Errorf("error loading plugins: %s", err)
	}

	// Load preloaded and external plugins
	if err := plugins.Initialize(); err != nil {
		return fmt.Errorf("error initializing plugins: %s", err)
	}

	if err := plugins.Inject(); err != nil {
		return fmt.Errorf("error initializing plugins: %s", err)
	}

	return nil
}

// connect to bootstrap peers (not sure yet what context this is used in)
func connectToPeers(ctx context.Context, ipfs icore.CoreAPI, peers []string) error {
	var wg sync.WaitGroup
	peerInfos := make(map[peer.ID]*peer.AddrInfo, len(peers))
	for _, addrStr := range peers {
		addr, err := ma.NewMultiaddr(addrStr)
		if err != nil {
			return err
		}
		pii, err := peer.AddrInfoFromP2pAddr(addr)
		if err != nil {
			return err
		}
		pi, ok := peerInfos[pii.ID]
		if !ok {
			pi = &peer.AddrInfo{ID: pii.ID}
			peerInfos[pi.ID] = pi
		}
		pi.Addrs = append(pi.Addrs, pii.Addrs...)
	}

	wg.Add(len(peerInfos))
	for _, peerInfo := range peerInfos {
		go func(peerInfo *peer.AddrInfo) {
			defer wg.Done()
			err := ipfs.Swarm().Connect(ctx, *peerInfo)
			if err != nil {
				log.Printf("failed to connect to %s: %s", peerInfo.ID, err)
			}
		}(peerInfo)
	}
	wg.Wait()
	return nil
}

// get a file or directory from unixfs
func getUnixfsNode(path string) (files.Node, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	f, err := files.NewSerialFile(path, false, st)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// write any given content to a given directory
func save(dir string, content files.Node, cid string) string {
	// if successful, write the content to temporary directory

	path := dir + cid
	err := files.WriteTo(content, path)
	if err != nil {
		panic(fmt.Errorf("could not write the fetched file to the temporary directory: %s", err))
	}

	fmt.Printf("save(): successfully saved content %s to disk location %s\n\n", cid, path)

	return path
}

var flagExp = flag.Bool("experimental", false, "enable experimental features")

// creates the repo using a config for the ipfs instance
func createRepo() (string, error) {
	dirPermissions := int(0777)
	repoPath := "/tmp/embedded-ipfs"

	if err := os.Mkdir(repoPath, os.FileMode(dirPermissions)); err == nil {
		fmt.Printf("creating repo directory %s with permissions 0777\n", repoPath)

		// Create a config with default options and a 2048 bit key
		cfg, err := config.Init(ioutil.Discard, 2048)
		if err != nil {
			return "", err
		}

		if *flagExp {
			// https://github.com/ipfs/go-ipfs/blob/master/docs/experimental-features.md#ipfs-filestore
			cfg.Experimental.FilestoreEnabled = true
			// https://github.com/ipfs/go-ipfs/blob/master/docs/experimental-features.md#ipfs-urlstore
			cfg.Experimental.UrlstoreEnabled = true
			// https://github.com/ipfs/go-ipfs/blob/master/docs/experimental-features.md#directory-sharding--hamt
			cfg.Experimental.ShardingEnabled = true
			// https://github.com/ipfs/go-ipfs/blob/master/docs/experimental-features.md#ipfs-p2p
			cfg.Experimental.Libp2pStreamMounting = true
			// https://github.com/ipfs/go-ipfs/blob/master/docs/experimental-features.md#p2p-http-proxy
			cfg.Experimental.P2pHttpProxy = true
			// https://github.com/ipfs/go-ipfs/blob/master/docs/experimental-features.md#strategic-providing
			cfg.Experimental.StrategicProviding = true
		}

		err = fsrepo.Init(repoPath, cfg)
		if err != nil {
			return "", fmt.Errorf("failed to init ephemeral node: %s", err)
		}

	} else {
		fmt.Print("repo already exists\n")
	}

	return repoPath, nil
}

// the create() determines the success of the spawnEmbedded(ctx) func & provides error handling
func create(ctx context.Context) iface.CoreAPI {
	if err := setupPlugins(""); err != nil {
		panic(fmt.Errorf("failed to setup plugins: %s", err))
	}

	// create a directory our IPFS instance (the repo)
	repoPath, err := createRepo()
	if err != nil {
		panic(fmt.Errorf("failed to create temp repo: %s", err))
	}

	// open the root directory for IPFS node (main repo)
	repo, err := fsrepo.Open(repoPath)
	if err != nil {
		panic(fmt.Errorf("failed to open repo: %s", err))
	}

	// construct the node
	nodeOptions := &node.BuildCfg{
		Online:  true,
		Routing: libp2p.DHTClientOption, // which option sets the node to be a client DHT node (only fetching records)
		Repo:    repo,
	}

	// assign constructed node to core IPFS node
	node, err := core.NewNode(ctx, nodeOptions)
	if err != nil {
		panic(fmt.Errorf("failed to create new IPFS node: %s", err))
	}

	c, err := coreapi.NewCoreAPI(node)
	if err != nil {
		panic(fmt.Errorf("failed to submit to god core: %s", err))
	}

	fmt.Printf("starting the embedded (ephemeral) IPFS node...\n\n")

	return c
}

func getFile(path string) files.Node {
	file, err := getUnixfsNode(path)
	if err != nil {
		panic(fmt.Errorf("failed to get file from unixfs: %s", err))
	}

	return file
}

// add some given content to a given IPFS instance
func add(ipfs iface.CoreAPI, ctx context.Context, file files.Node) icorepath.Resolved {
	// add the content to embedded ipfs instance
	content, err := ipfs.Unixfs().Add(ctx, file)
	if err != nil {
		panic(fmt.Errorf("failed to add content: %s", err))
	}

	fmt.Printf("add(): successfully added content to the embedded IPFS instance w/ a path of: %s\n", content.String())

	return content
}

// from the given IPFS instance, fetch any given content from the IPFS network
func fetch(ipfs iface.CoreAPI, ctx context.Context, cid string) icorepath.Resolved {
	bootstrapNodes := []string{}

	// go routine for connecting to peers using the context, our embedded ipfs instance, and the provided local bootstrap nodes list
	go func() {
		err := connectToPeers(ctx, ipfs, bootstrapNodes)
		if err != nil {
			log.Printf("failed connect to peers: %s", err)
		}
	}()

	fmt.Printf("fetch(): fetching given content from the IPFS network w/ a CID of: %s\n", cid)
	path := icorepath.New(cid)

	content, err := ipfs.Unixfs().Get(ctx, path)
	if err != nil {
		panic(fmt.Errorf("could not get contents of CID: %s", err))
	}

	resolvedContent := add(ipfs, ctx, content)
	fmt.Printf("added content\n")

	return resolvedContent
}

// from the given IPFS instance, fetch any given content from the IPFS network
func get(ipfs iface.CoreAPI, ctx context.Context, cid string) files.Node {
	fmt.Printf("get(): getting file from IPFS instance: %s\n", cid)

	path := icorepath.New(cid)

	content, err := ipfs.Unixfs().Get(ctx, path)
	if err != nil {
		panic(fmt.Errorf("could not get contents of CID: %s", err))
	}

	return content
}

// from the given IPFS instance, fetch any given content from the IPFS network
func cat(ipfs iface.CoreAPI, ctx context.Context, cid string) []byte {
	path := icorepath.New(cid)

	fmt.Printf("getting content %s from path /tmp/ipfs-embedded%s \n", cid, path)

	contents, err := ipfs.Block().Get(ctx, path)
	if err != nil {
		panic(fmt.Errorf("failed to get contents from IPFS node: %s", err))
	}

	buffer, err := ioutil.ReadAll(contents)
	if err != nil {
		panic(fmt.Errorf("failed to read contents: %s", err))
	}

	return buffer
}
