#compdef kue
# zsh completion for kue
#
# Install: put this file on your $fpath as `_kue`, e.g.
#   kue completion zsh > "${fpath[1]}/_kue"
# then restart your shell (compinit must run in ~/.zshrc).

_kue() {
    local curcontext="$curcontext" state line
    typeset -A opt_args

    local -a commands
    commands=(
        'run:Execute a workflow (file, org/name, or project dir)'
        'modules:List packages compiled into this kue binary'
        'transitions:List every action a workflow can call here'
        'create:Create a new application/project from a template'
        'add:Add a module, workflow, feature, solution, package, or transition'
        'update:Regenerate module cache (di.go, meta.go, modules.json)'
        'show:List apps, solutions, features, workflows, modules in the project'
        'inspect:Show a workflow source, provenance, dependencies, and actions'
        'wsl:Search the workflow registry (kue wsl <query>)'
        'get:Fetch a workflow + its deps into the project (idempotent)'
        'upload:Publish local workflows (all, or a name/glob)'
        'list:List workflows owned by the authenticated user'
        'status:Compare local workflows against the server'
        'delete:Delete a workflow on the server'
        'register:Register a new Kuetix account'
        'login:Authenticate with the Kuetix API'
        'logout:Remove the stored Kuetix login payload'
        'whoami:Show the currently authenticated identity'
        'profile:Manage the authenticated user profile'
        'install:Download a workflow or package'
        'package:Manage package dependencies and publishing'
        'completion:Output a shell completion script'
        'templates:Manage the .kue/templates cache'
        'help:Show help for kue or a command'
    )

    _arguments -C \
        '(-v --verbose)'{-v,--verbose}'[Verbose output]' \
        '(-q --quiet)'{-q,--quiet}'[Quiet mode]' \
        '(-h --help)'{-h,--help}'[Show help]' \
        '--debug[Debug output]' \
        '(-H --host)'{-H,--host}'[API host]:host:' \
        '(-c --config)'{-c,--config}'[Config file]:file:_files' \
        '--modules[Modules directory]:dir:_files -/' \
        '--workflows[Workflows directory]:dir:_files -/' \
        '1: :->command' \
        '*:: :->args' \
        && return 0

    case $state in
        command)
            _describe -t commands 'kue command' commands
            ;;
        args)
            case $words[1] in
                add)
                    _values 'subcommand' module workflow feature solution package transition
                    ;;
                wsl)
                    _values 'subcommand' get inspect status upload list delete
                    ;;
                package)
                    _values 'subcommand' add update search install list list-local enable publish show
                    ;;
                profile)
                    _values 'subcommand' get update
                    ;;
                templates)
                    _values 'subcommand' download update clear status
                    ;;
                completion)
                    _values 'shell' bash zsh
                    ;;
                help)
                    _values 'topic' examples env
                    ;;
                run|r|install|i|inspect|upload)
                    _alternative \
                        'files:workflow file:_files -g "*.(wsl|swsl)"' \
                        'dirs:project directory:_files -/'
                    ;;
            esac
            ;;
    esac
}

_kue "$@"
