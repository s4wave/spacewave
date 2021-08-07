import * as React from 'react'

import GeoPattern from 'geopattern'
import randomString from 'random-string'

import {
  Alert,
  AlertIcon,
  AlertTitle,
  AlertDescription,
  Flex,
  Input,
  Button,
  InputGroup,
  Stack,
  InputLeftElement,
  Box,
  Link,
  FormControl,
  FormHelperText,
  Progress,
} from '@chakra-ui/react'
import { FaUserAlt, FaLock } from 'react-icons/fa'

interface ILoginProps {}

interface ILoginState {
  // username is the username field
  username: string
  // password is the password field
  password: string
  // authenticating indicates the system is currently processing the auth.
  authenticating: boolean
  // errorMessage is the login error message.
  errorMessage?: string
  // bgImage is the background image url
  bgImage: string
}

// Login shows a user-based authentication interface.
export default class Login extends React.Component<ILoginProps, ILoginState> {
  // bgSeed is the background seed for when the username is empty
  private bgSeed: string

  constructor(props: ILoginProps) {
    super(props)

    this.bgSeed = randomString()
    const bgImage = this.renderBackground()
    this.state = {
      bgImage: bgImage,
      username: '',
      password: '',
      authenticating: false,
    }

    this.handleUsernameBlur = this.handleUsernameBlur.bind(this)
    this.handleUsernameChange = this.handleUsernameChange.bind(this)
    this.handlePasswordChange = this.handlePasswordChange.bind(this)
    this.handleSubmit = this.handleSubmit.bind(this)
  }

  /* <div className="login-logo">APERTURE</div> */
  public render() {
    const loginDisabled =
      this.state.authenticating ||
      !this.state.username.length ||
      !this.state.password.length

    return (
      <Flex
        flexDirection="column"
        width="100wh"
        height="100vh"
        background={this.state.bgImage}
        justifyContent="center"
        alignItems="center"
      >
        <Stack
          flexDir="column"
          mb="2"
          justifyContent="center"
          alignItems="center"
        >
          Aperture
          <Alert status="error" opacity={!!this.state.errorMessage ? 100 : 0}>
            <AlertIcon />
            <AlertTitle mr={2}>Login failed!</AlertTitle>
            <AlertDescription>{this.state.errorMessage}</AlertDescription>
          </Alert>
          <Box minW={{ base: '90%', md: '468px' }}>
            <form
              className="login-form-fields"
              onSubmit={loginDisabled ? null : this.handleSubmit}
            >
              <Stack
                spacing={4}
                p="1rem"
                backgroundColor="whiteAlpha.900"
                boxShadow="md"
              >
                <FormControl>
                  <InputGroup>
                    <InputLeftElement
                      pointerEvents="none"
                      children={<FaUserAlt color="gray.300" />}
                    />
                    <Input
                      label="username"
                      className="login-field"
                      value={this.state.username}
                      onBlur={this.handleUsernameBlur}
                      onChange={this.handleUsernameChange}
                      disabled={this.state.authenticating}
                      isInvalid={!!this.state.errorMessage}
                    />
                  </InputGroup>
                </FormControl>
                <FormControl>
                  <InputGroup>
                    <InputLeftElement
                      pointerEvents="none"
                      color="gray.300"
                      children={<FaLock color="gray.300" />}
                    />
                    <Input
                      label="password"
                      type="password"
                      className="login-field"
                      value={this.state.password}
                      onChange={this.handlePasswordChange}
                      disabled={this.state.authenticating}
                      isInvalid={!!this.state.errorMessage}
                    />
                  </InputGroup>
                  <FormHelperText textAlign="right">
                    <Link>forgot password?</Link>
                  </FormHelperText>
                </FormControl>
                <Button
                  borderRadius={0}
                  type="submit"
                  variant="solid"
                  colorScheme="teal"
                  width="full"
                  disabled={loginDisabled}
                >
                  Login
                </Button>
                <Progress
                  isIndeterminate={true}
                  opacity={this.state.authenticating ? 100 : 0}
                />
              </Stack>
            </form>
          </Box>
        </Stack>
      </Flex>
    )

    /*
      <Box>
        No account?{' '}
        <Link color="teal.500" href="#">
          Sign Up
        </Link>
      </Box>
      */
  }

  // handleSubmit handles the form submission event.
  private handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    this.setState({ authenticating: true })

    // if (!this.props.session) {
    setTimeout(() => {
      this.setState({ authenticating: false, errorMessage: 'No session.' })
    }, 3000)
    return

    /*
    console.log('authenticating')
    this.props.session
      .authenticate(
        this.state.username,
        this.state.password,
        (status: string) => {
          console.log('authenticating state: ' + status)
          this.setState({ errorMessage: status })
        }
      )
      .then((success: boolean) => {
        console.log('authenticating success: ' + success)
        if (!success) {
          this.setState({ authenticating: false })
        }
      })
      */
  }

  // renderBackground renders the background
  private renderBackground(): string {
    let seed = this.bgSeed
    if (this.state && this.state.username && this.state.username.length) {
      seed = this.state.username
    }

    const pattern = GeoPattern.generate(seed, {})
    return pattern.toDataUrl()
  }

  private handleUsernameChange(e: React.ChangeEvent<HTMLInputElement>) {
    const username: string = e.target.value
    this.setState({ username })
  }

  private handlePasswordChange(e: React.ChangeEvent<HTMLInputElement>) {
    const password: string = e.target.value
    this.setState({ password })
  }

  private handleUsernameBlur(_e: React.FocusEvent<HTMLInputElement>) {
    this.setState({ bgImage: this.renderBackground(), errorMessage: undefined })
  }
}
