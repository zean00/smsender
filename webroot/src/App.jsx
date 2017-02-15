import React, {Component} from 'react'
import {Provider, observer} from 'mobx-react'
import DevTools from 'mobx-react-devtools'
import {RouterStore, syncHistoryWithStore} from 'mobx-react-router'
import {Router, IndexRoute, Route, browserHistory} from 'react-router'
import MuiThemeProvider from 'material-ui/styles/MuiThemeProvider'
import lightBaseTheme from 'material-ui/styles/baseThemes/lightBaseTheme'
import getMuiTheme from 'material-ui/styles/getMuiTheme'

import Home from './components/Home'
import Console from './components/Console'
import RouterPage from './components/router/RouterPage'
import SMSPage from './components/sms/SMSPage'
import SendPage from './components/sms/SendPage'
import DetailsPage from './components/sms/DetailsPage'

const routingStore = new RouterStore()

const stores = {
    routing: routingStore
}

const history = syncHistoryWithStore(browserHistory, routingStore)

export default class App extends Component {
    render() {
        return (
            <MuiThemeProvider muiTheme={getMuiTheme(lightBaseTheme)}>
                <div>
                    <Provider {...stores}>
                        <Router history={history}>
                            <Route path="/" component={Home} />
                            <Route path="console" component={Console}>
                                <Route path="sms" component={SMSPage} />
                                <Route path="sms/send" component={SendPage} />
                                <Route path="router" component={RouterPage} />
                            </Route>
                        </Router>
                    </Provider>
                    {module.hot ? <DevTools /> : null}
                </div>
            </MuiThemeProvider>
        )
    }
}
