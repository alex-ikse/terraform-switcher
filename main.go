package main

/*
* Version 0.12.0
* Compatible with Mac OS X AND other LINUX OS ONLY
 */

/*** OPERATION WORKFLOW ***/
/*
* 1- Create /usr/local/terraform directory if does not exist
* 2- Download zip file from url to /usr/local/terraform
* 3- Unzip the file to /usr/local/terraform
* 4- Rename the file from `terraform` to `terraform_version`
* 5- Remove the downloaded zip file
* 6- Read the existing symlink for terraform (Check if it's a homebrew symlink)
* 7- Remove that symlink (Check if it's a homebrew symlink)
* 8- Create new symlink to binary  `terraform_version`
 */

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	semver "github.com/hashicorp/go-version"
	"github.com/hashicorp/hcl2/gohcl"
	"github.com/hashicorp/hcl2/hclparse"
	"github.com/hashicorp/terraform-config-inspect/tfconfig"

	"github.com/manifoldco/promptui"
	"github.com/pborman/getopt"
	"github.com/spf13/viper"

	lib "github.com/warrensbox/terraform-switcher/lib"
)

const (
	defaultMirror = "https://releases.hashicorp.com/terraform"
	defaultBin    = "/usr/local/bin/terraform" //default bin installation dir
	defaultLatest = ""
	tfvFilename   = ".terraform-version"
	rcFilename    = ".tfswitchrc"
	tomlFilename  = ".tfswitch.toml"
	tgHclFilename = "terragrunt.hcl"
	versionPrefix = "terraform_"
)

var version = "0.12.0\n"

func main() {
	tfArch := getopt.StringLong("arch", 'a', runtime.GOARCH, "Force the architecture the terraform binary. Ex: tfswitch -a "+runtime.GOARCH)
	custBinPath := getopt.StringLong("bin", 'b', lib.ConvertExecutableExt(defaultBin), "Custom binary path. Ex: tfswitch -b "+lib.ConvertExecutableExt("/Users/username/bin/terraform"))
	listAllFlag := getopt.BoolLong("list-all", 'l', "List all versions of terraform - including beta and rc")
	latestPre := getopt.StringLong("latest-pre", 'p', defaultLatest, "Latest pre-release implicit version. Ex: tfswitch --latest-pre 0.13 downloads 0.13.0-rc1 (latest)")
	showLatestPre := getopt.StringLong("show-latest-pre", 'P', defaultLatest, "Show latest pre-release implicit version. Ex: tfswitch --show-latest-pre 0.13 prints 0.13.0-rc1 (latest)")
	latestStable := getopt.StringLong("latest-stable", 's', defaultLatest, "Latest implicit version. Ex: tfswitch --latest-stable 0.13 downloads 0.13.7 (latest)")
	showLatestStable := getopt.StringLong("show-latest-stable", 'S', defaultLatest, "Show latest implicit version. Ex: tfswitch --show-latest-stable 0.13 prints 0.13.7 (latest)")
	latestFlag := getopt.BoolLong("latest", 'u', "Get latest stable version")
	showLatestFlag := getopt.BoolLong("show-latest", 'U', "Show latest stable version")
	mirrorURL := getopt.StringLong("mirror", 'm', defaultMirror, "Install from a remote other than the default. Default: https://releases.hashicorp.com/terraform")
	chDirPath := getopt.StringLong("chdir", 'c', "", "Switch to a different working directory before executing the given command. Ex: tfswitch --chdir terraform_project will run tfswitch in the terraform_project directory")
	versionFlag := getopt.BoolLong("version", 'v', "Displays the version of tfswitch")
	helpFlag := getopt.BoolLong("help", 'h', "Displays help message")
	_ = versionFlag

	getopt.Parse()
	args := getopt.Args()

	dir := lib.GetCurrentDirectory()
	homedir := lib.GetHomeDirectory()

	if *chDirPath != "" {
		dir = dir + "/" + *chDirPath
	}

	TFVersionFile := filepath.Join(dir, tfvFilename)           //settings for .terraform-version file in current directory (tfenv compatible)
	RCFile := filepath.Join(dir, rcFilename)                   //settings for .tfswitchrc file in current directory (backward compatible purpose)
	TOMLConfigFile := filepath.Join(dir, tomlFilename)         //settings for .tfswitch.toml file in current directory (option to specify bin directory)
	HomeTOMLConfigFile := filepath.Join(homedir, tomlFilename) //settings for .tfswitch.toml file in home directory (option to specify bin directory)
	TGHACLFile := filepath.Join(dir, tgHclFilename)            //settings for terragrunt.hcl file in current directory (option to specify bin directory)

	switch {
	case *versionFlag:
		//if *versionFlag {
		fmt.Printf("\nVersion: %v\n", version)
	case *helpFlag:
		//} else if *helpFlag {
		usageMessage()
	/* Checks if the .tfswitch.toml file exist in home or current directory
	 * This block checks to see if the tfswitch toml file is provided in the current path.
	 * If the .tfswitch.toml file exist, it has a higher precedence than the .tfswitchrc file
	 * You can specify the custom binary path and the version you desire
	 * If you provide a custom binary path with the -b option, this will override the bin value in the toml file
	 * If you provide a version on the command line, this will override the version value in the toml file
	 */
	case fileExists(TOMLConfigFile) || fileExists(HomeTOMLConfigFile):
		version := ""
		binPath := *custBinPath
		if fileExists(TOMLConfigFile) { //read from toml from current directory
			version, binPath = getParamsTOML(binPath, dir)
		} else { // else read from toml from home directory
			version, binPath = getParamsTOML(binPath, homedir)
		}

		switch {
		/* GIVEN A TOML FILE, */
		/* show all terraform version including betas and RCs*/
		case *listAllFlag:
			listAll := true //set list all true - all versions including beta and rc will be displayed
			installOption(tfArch, listAll, &binPath, mirrorURL)
		/* latest pre-release implicit version. Ex: tfswitch --latest-pre 0.13 downloads 0.13.0-rc1 (latest) */
		case *latestPre != "":
			preRelease := true
			installLatestImplicitVersion(tfArch, *latestPre, custBinPath, mirrorURL, preRelease)
		/* latest implicit version. Ex: tfswitch --latest 0.13 downloads 0.13.5 (latest) */
		case *latestStable != "":
			preRelease := false
			installLatestImplicitVersion(tfArch, *latestStable, custBinPath, mirrorURL, preRelease)
		/* latest stable version */
		case *latestFlag:
			installLatestVersion(tfArch, custBinPath, mirrorURL)
		/* version provided on command line as arg */
		case len(args) == 1:
			installVersion(tfArch, args[0], &binPath, mirrorURL)
		/* provide an tfswitchrc file (IN ADDITION TO A TOML FILE) */
		case fileExists(RCFile) && len(args) == 0:
			readingFileMsg(rcFilename)
			tfversion := retrieveFileContents(RCFile)
			installVersion(tfArch, tfversion, &binPath, mirrorURL)
		/* if .terraform-version file found (IN ADDITION TO A TOML FILE) */
		case fileExists(TFVersionFile) && len(args) == 0:
			readingFileMsg(tfvFilename)
			tfversion := retrieveFileContents(TFVersionFile)
			installVersion(tfArch, tfversion, &binPath, mirrorURL)
		/* if versions.tf file found (IN ADDITION TO A TOML FILE) */
		case checkTFModuleFileExist(dir) && len(args) == 0:
			installTFProvidedModule(tfArch, dir, &binPath, mirrorURL)
		/* if Terraform Version environment variable is set */
		case checkTFEnvExist() && len(args) == 0 && version == "":
			tfversion := os.Getenv("TF_VERSION")
			fmt.Printf("Terraform version environment variable: %s\n", tfversion)
			installVersion(tfArch, tfversion, custBinPath, mirrorURL)
		/* if terragrunt.hcl file found (IN ADDITION TO A TOML FILE) */
		case fileExists(TGHACLFile) && checkVersionDefinedHCL(&TGHACLFile) && len(args) == 0:
			installTGHclFile(tfArch, &TGHACLFile, &binPath, mirrorURL)
		// if no arg is provided - but toml file is provided
		case version != "":
			installVersion(tfArch, version, &binPath, mirrorURL)
		default:
			listAll := false //set list all false - only official release will be displayed
			installOption(tfArch, listAll, &binPath, mirrorURL)
		}

	case *tfArch != "arm" && *tfArch != "arm64" && *tfArch != "386" && *tfArch != "amd64":
		fmt.Printf("\nInvalid architecture: %s. Valid values are arm, arm64, 386 and amd64\n", *tfArch)
		os.Exit(1)

	/* show all terraform version including betas and RCs*/
	case *listAllFlag:
		installWithListAll(tfArch, custBinPath, mirrorURL)

	/* latest pre-release implicit version. Ex: tfswitch --latest-pre 0.13 downloads 0.13.0-rc1 (latest) */
	case *latestPre != "":
		preRelease := true
		installLatestImplicitVersion(tfArch, *latestPre, custBinPath, mirrorURL, preRelease)

	/* show latest pre-release implicit version. Ex: tfswitch --latest-pre 0.13 downloads 0.13.0-rc1 (latest) */
	case *showLatestPre != "":
		preRelease := true
		showLatestImplicitVersion(*showLatestPre, custBinPath, mirrorURL, preRelease)

	/* latest implicit version. Ex: tfswitch --latest 0.13 downloads 0.13.5 (latest) */
	case *latestStable != "":
		preRelease := false
		installLatestImplicitVersion(tfArch, *latestStable, custBinPath, mirrorURL, preRelease)

	/* show latest implicit stable version. Ex: tfswitch --latest 0.13 downloads 0.13.5 (latest) */
	case *showLatestStable != "":
		preRelease := false
		showLatestImplicitVersion(*showLatestStable, custBinPath, mirrorURL, preRelease)

	/* latest stable version */
	case *latestFlag:
		installLatestVersion(tfArch, custBinPath, mirrorURL)

	/* show latest stable version */
	case *showLatestFlag:
		showLatestVersion(custBinPath, mirrorURL)

	/* version provided on command line as arg */
	case len(args) == 1:
		installVersion(tfArch, args[0], custBinPath, mirrorURL)

	/* provide an tfswitchrc file */
	case fileExists(RCFile) && len(args) == 0:
		readingFileMsg(rcFilename)
		tfversion := retrieveFileContents(RCFile)
		installVersion(tfArch, tfversion, custBinPath, mirrorURL)

	/* if .terraform-version file found */
	case fileExists(TFVersionFile) && len(args) == 0:
		readingFileMsg(tfvFilename)
		tfversion := retrieveFileContents(TFVersionFile)
		installVersion(tfArch, tfversion, custBinPath, mirrorURL)

	/* if versions.tf file found */
	case checkTFModuleFileExist(dir) && len(args) == 0:
		installTFProvidedModule(tfArch, dir, custBinPath, mirrorURL)

	/* if terragrunt.hcl file found */
	case fileExists(TGHACLFile) && checkVersionDefinedHCL(&TGHACLFile) && len(args) == 0:
		installTGHclFile(tfArch, &TGHACLFile, custBinPath, mirrorURL)

	/* if Terraform Version environment variable is set */
	case checkTFEnvExist() && len(args) == 0:
		tfversion := os.Getenv("TF_VERSION")
		fmt.Printf("Terraform version environment variable: %s\n", tfversion)
		installVersion(tfArch, tfversion, custBinPath, mirrorURL)

	// if no arg is provided
	default:
		listAll := false //set list all false - only official release will be displayed
		installOption(tfArch, listAll, custBinPath, mirrorURL)
	}
}

/* Helper functions */

// install with all possible versions, including beta and rc
func installWithListAll(tfArch *string, custBinPath, mirrorURL *string) {
	listAll := true //set list all true - all versions including beta and rc will be displayed
	installOption(tfArch, listAll, custBinPath, mirrorURL)
}

// install latest stable tf version
func installLatestVersion(tfArch *string, custBinPath, mirrorURL *string) {
	tfversion, _ := lib.GetTFLatest(*mirrorURL)
	lib.Install(tfArch, tfversion, *custBinPath, *mirrorURL)
}

// show install latest stable tf version
func showLatestVersion(custBinPath, mirrorURL *string) {
	tfversion, _ := lib.GetTFLatest(*mirrorURL)
	fmt.Printf("%s\n", tfversion)
}

// install latest - argument (version) must be provided
func installLatestImplicitVersion(tfArch *string, requestedVersion string, custBinPath, mirrorURL *string, preRelease bool) {
	_, err := semver.NewConstraint(requestedVersion)
	if err != nil {
		fmt.Printf("error parsing constraint: %s\n", err)
	}
	//if lib.ValidMinorVersionFormat(requestedVersion) {
	tfversion, err := lib.GetTFLatestImplicit(*mirrorURL, preRelease, requestedVersion)
	if err == nil && tfversion != "" {
		lib.Install(tfArch, tfversion, *custBinPath, *mirrorURL)
	}
	fmt.Printf("Error parsing constraint: %s\n", err)
	lib.PrintInvalidMinorTFVersion()
}

// show latest - argument (version) must be provided
func showLatestImplicitVersion(requestedVersion string, custBinPath, mirrorURL *string, preRelease bool) {
	if lib.ValidMinorVersionFormat(requestedVersion) {
		tfversion, _ := lib.GetTFLatestImplicit(*mirrorURL, preRelease, requestedVersion)
		if len(tfversion) > 0 {
			fmt.Printf("%s\n", tfversion)
		} else {
			fmt.Println("The provided terraform version does not exist. Try `tfswitch -l` to see all available versions.")
			os.Exit(1)
		}
	} else {
		lib.PrintInvalidMinorTFVersion()
	}
}

// install with provided version as argument
func installVersion(tfArch *string, arg string, custBinPath *string, mirrorURL *string) {
	if lib.ValidVersionFormat(arg) {
		requestedVersion := arg

		//check to see if the requested version has been downloaded before
		installLocation := lib.GetInstallLocation()
		installFileVersionPath := lib.ConvertExecutableExt(filepath.Join(installLocation, versionPrefix+requestedVersion))
		recentDownloadFile := lib.CheckFileExist(installFileVersionPath)
		if recentDownloadFile {
			lib.ChangeSymlink(installFileVersionPath, *custBinPath)
			fmt.Printf("Switched terraform to version %q (%s)\n", requestedVersion, *tfArch)
			lib.AddRecent(requestedVersion + "-" + *tfArch) //add to recent file for faster lookup
			os.Exit(0)
		}

		//if the requested version had not been downloaded before
		listAll := true                                     //set list all true - all versions including beta and rc will be displayed
		tflist, _ := lib.GetTFList(*mirrorURL, listAll)     //get list of versions
		exist := lib.VersionExist(requestedVersion, tflist) //check if version exist before downloading it

		if exist {
			lib.Install(tfArch, requestedVersion, *custBinPath, *mirrorURL)
		} else {
			fmt.Println("The provided terraform version does not exist. Try `tfswitch -l` to see all available versions.")
			os.Exit(1)
		}

	} else {
		lib.PrintInvalidTFVersion()
		fmt.Println("Args must be a valid terraform version")
		usageMessage()
		os.Exit(1)
	}
}

//retrive file content of regular file
func retrieveFileContents(file string) string {
	fileContents, err := ioutil.ReadFile(file)
	if err != nil {
		fmt.Printf("Failed to read %s file. Follow the README.md instructions for setup. https://github.com/warrensbox/terraform-switcher/blob/master/README.md\n", tfvFilename)
		fmt.Printf("Error: %s\n", err)
		os.Exit(1)
	}
	tfversion := strings.TrimSuffix(string(fileContents), "\n")
	return tfversion
}

// Print message reading file content of :
func readingFileMsg(filename string) {
	fmt.Printf("Reading file %s \n", filename)
}

// fileExists checks if a file exists and is not a directory before we try using it to prevent further errors.
func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

// fileExists checks if a file exists and is not a directory before we
// try using it to prevent further errors.
func checkTFModuleFileExist(dir string) bool {

	module, _ := tfconfig.LoadModule(dir)
	if len(module.RequiredCore) >= 1 {
		return true
	}
	return false
}

// checkTFEnvExist - checks if the TF_VERSION environment variable is set
func checkTFEnvExist() bool {
	tfversion := os.Getenv("TF_VERSION")
	if tfversion != "" {
		return true
	}
	return false
}

/* parses everything in the toml file, return required version and bin path */
func getParamsTOML(binPath string, dir string) (string, string) {
	path := lib.GetHomeDirectory()
	if dir == path {
		path = "home directory"
	} else {
		path = "current directory"
	}
	fmt.Printf("Reading configuration from %s\n", path+" for "+tomlFilename) //takes the default bin (defaultBin) if user does not specify bin path
	configfileName := lib.GetFileName(tomlFilename)                          //get the config file
	viper.SetConfigType("toml")
	viper.SetConfigName(configfileName)
	viper.AddConfigPath(dir)

	errs := viper.ReadInConfig() // Find and read the config file
	if errs != nil {
		fmt.Printf("Unable to read %s provided\n", tomlFilename) // Handle errors reading the config file
		fmt.Println(errs)
		os.Exit(1) // exit immediately if config file provided but it is unable to read it
	}

	bin := viper.Get("bin")                                            // read custom binary location
	if binPath == lib.ConvertExecutableExt(defaultBin) && bin != nil { // if the bin path is the same as the default binary path and if the custom binary is provided in the toml file (use it)
		binPath = os.ExpandEnv(bin.(string))
	}
	//fmt.Println(binPath) //uncomment this to debug
	version := viper.Get("version") //attempt to get the version if it's provided in the toml
	if version == nil {
		version = ""
	}

	return version.(string), binPath
}

func usageMessage() {
	fmt.Print("\n\n")
	getopt.PrintUsage(os.Stderr)
	fmt.Println("Supply the terraform version as an argument, or choose from a menu")
}

/* installOption : displays & installs tf version */
/* listAll = true - all versions including beta and rc will be displayed */
/* listAll = false - only official stable release are displayed */
func installOption(tfArch *string, listAll bool, custBinPath, mirrorURL *string) {
	tflist, _ := lib.GetTFList(*mirrorURL, listAll) //get list of versions
	recentVersions, _ := lib.GetRecentVersions()    //get recent versions from RECENT file
	tflist = append(recentVersions, tflist...)      //append recent versions to the top of the list
	tflist = lib.RemoveDuplicateVersions(tflist)    //remove duplicate version

	if len(tflist) == 0 {
		fmt.Println("[ERROR] : List is empty")
		os.Exit(1)
	}
	/* prompt user to select version of terraform */
	prompt := promptui.Select{
		Label: "Select Terraform version",
		Items: tflist,
	}

	_, tfversion, errPrompt := prompt.Run()

	tfversion = strings.Trim(tfversion, " *recent") //trim versions with the string " *recent" appended

	if strings.Contains(tfversion, "-") {
		tfversionSplitted := strings.SplitN(tfversion, "-", 2)
		tfversion = tfversionSplitted[0]
		tfArch = &tfversionSplitted[1]
	}

	if errPrompt != nil {
		log.Printf("Prompt failed %v\n", errPrompt)
		os.Exit(1)
	}

	lib.Install(tfArch, tfversion, *custBinPath, *mirrorURL)
	os.Exit(0)
}

// install when tf file is provided
func installTFProvidedModule(tfArch *string, dir string, custBinPath, mirrorURL *string) {
	fmt.Printf("Reading required version from terraform file\n")
	module, _ := tfconfig.LoadModule(dir)
	tfconstraint := module.RequiredCore[0] //we skip duplicated definitions and use only first one
	installFromConstraint(tfArch, &tfconstraint, custBinPath, mirrorURL)
}

// install using a version constraint
func installFromConstraint(tfArch *string, tfconstraint *string, custBinPath, mirrorURL *string) {

	tfversion, err := lib.GetSemver(tfconstraint, mirrorURL)
	if err == nil {
		lib.Install(tfArch, tfversion, *custBinPath, *mirrorURL)
	}
	fmt.Println(err)
	fmt.Println("No version found to match constraint. Follow the README.md instructions for setup. https://github.com/warrensbox/terraform-switcher/blob/master/README.md")
	os.Exit(1)
}

// Install using version constraint from terragrunt file
func installTGHclFile(tfArch *string, tgFile *string, custBinPath, mirrorURL *string) {
	fmt.Printf("Terragrunt file found: %s\n", *tgFile)
	parser := hclparse.NewParser()
	file, diags := parser.ParseHCLFile(*tgFile) //use hcl parser to parse HCL file
	if diags.HasErrors() {
		fmt.Println("Unable to parse HCL file")
		os.Exit(1)
	}
	var version terragruntVersionConstraints
	gohcl.DecodeBody(file.Body, nil, &version)
	installFromConstraint(tfArch, &version.TerraformVersionConstraint, custBinPath, mirrorURL)
}

type terragruntVersionConstraints struct {
	TerraformVersionConstraint string `hcl:"terraform_version_constraint"`
}

// check if version is defined in hcl file /* lazy-emergency fix - will improve later */
func checkVersionDefinedHCL(tgFile *string) bool {
	parser := hclparse.NewParser()
	file, diags := parser.ParseHCLFile(*tgFile) //use hcl parser to parse HCL file
	if diags.HasErrors() {
		fmt.Println("Unable to parse HCL file")
		os.Exit(1)
	}
	var version terragruntVersionConstraints
	gohcl.DecodeBody(file.Body, nil, &version)
	if version == (terragruntVersionConstraints{}) {
		return false
	}
	return true
}
